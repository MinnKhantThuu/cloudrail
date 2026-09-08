package deployment

import (
	"cloudrail/internal/secrets"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"cloudrail/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	DB     *pgxpool.Pool
	Cipher *secrets.Cipher
}

func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 6
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = migrations.Apply(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}
func (s *Store) CreateProject(ctx context.Context, name string) (Project, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback(ctx)
	p := Project{ID: ID(), Name: name}
	err = tx.QueryRow(ctx, `INSERT INTO projects(id,name) VALUES($1,$2) RETURNING created_at`, p.ID, p.Name).Scan(&p.CreatedAt)
	if err != nil {
		return p, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO environments(project_id,name) VALUES($1,'production')`, p.ID)
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}
func (s *Store) CreateService(ctx context.Context, projectID, name string, environment ...string) (Service, error) {
	env := "production"
	if len(environment) > 0 && environment[0] != "" {
		env = environment[0]
	}
	v := Service{ID: ID(), ProjectID: projectID, Name: name, Environment: env, DesiredState: "running"}
	domain := os.Getenv("APP_DOMAIN")
	if domain == "" {
		domain = "localhost"
	}
	v.Host = v.ID + "." + domain
	v.Settings = Settings{Kind: "http", MemoryMB: 256, CPUMillis: 1000, Network: PrivateNetwork(projectID, env)}
	settings, _ := json.Marshal(v.Settings)
	err := s.DB.QueryRow(ctx, `INSERT INTO services(id,project_id,name,host,environment,settings) VALUES($1,$2,$3,$4,$5,$6) RETURNING created_at`, v.ID, projectID, name, v.Host, env, settings).Scan(&v.CreatedAt)
	if v.Settings.Kind != "postgres" {
		v.URL = "http://" + v.Host + ":8088"
		if os.Getenv("PUBLIC_MODE") == "true" {
			v.URL = "https://" + v.Host
		}
	}
	return v, err
}

const depColumns = `id,service_id,image,port,health_path,status,error,logs,created_at,updated_at,settings`

func scanDep(row pgx.Row) (Deployment, error) {
	var d Deployment
	var raw []byte
	err := row.Scan(&d.ID, &d.ServiceID, &d.Image, &d.Port, &d.HealthPath, &d.Status, &d.Error, &d.Logs, &d.CreatedAt, &d.UpdatedAt, &raw)
	if err == nil {
		err = json.Unmarshal(raw, &d.Settings)
	}
	return d, err
}
func scanSvc(row pgx.Row) (Service, error) {
	var v Service
	var raw []byte
	err := row.Scan(&v.ID, &v.ProjectID, &v.Name, &v.Environment, &v.Host, &v.ActiveID, &v.CreatedAt, &v.DesiredState, &raw)
	if err == nil {
		err = json.Unmarshal(raw, &v.Settings)
	}
	if v.Settings.Kind != "postgres" {
		v.URL = "http://" + v.Host + ":8088"
		if os.Getenv("PUBLIC_MODE") == "true" {
			v.URL = "https://" + v.Host
		}
	}
	return v, err
}
func (s *Store) Enqueue(ctx context.Context, serviceID string, spec Spec, keys ...string) (Deployment, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Deployment{}, err
	}
	defer tx.Rollback(ctx)
	d, err := s.enqueueTx(ctx, tx, serviceID, spec, keys...)
	if err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}
func (s *Store) enqueueTx(ctx context.Context, tx pgx.Tx, serviceID string, spec Spec, keys ...string) (Deployment, error) {
	var err error
	// Locking the owning service makes the per-service queue limit race-safe.
	var owner string
	var settings []byte
	err = tx.QueryRow(ctx, `SELECT id,settings FROM services WHERE id=$1 FOR UPDATE`, serviceID).Scan(&owner, &settings)
	if err != nil {
		return Deployment{}, err
	}
	var config Settings
	if err = json.Unmarshal(settings, &config); err != nil {
		return Deployment{}, err
	}
	if config.Kind == "postgres" && (spec.Image != PostgresImage || spec.Port != 5432 || spec.HealthPath != "/") {
		return Deployment{}, errors.New("PostgreSQL template upgrades require a tested migration; use the pinned template image")
	}
	key := ""
	if len(keys) > 0 {
		key = keys[0]
	}
	if len(key) > 128 {
		return Deployment{}, ErrConflict
	}
	payload, _ := json.Marshal(spec)
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	if key != "" {
		prior, e := scanDep(tx.QueryRow(ctx, `SELECT `+depColumns+` FROM deployments WHERE service_id=$1 AND request_key=$2`, serviceID, key))
		if e == nil {
			var oldHash string
			if e = tx.QueryRow(ctx, `SELECT request_hash FROM deployments WHERE id=$1`, prior.ID).Scan(&oldHash); e != nil {
				return prior, e
			}
			if oldHash != hash {
				return prior, ErrConflict
			}
			return prior, nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return Deployment{}, e
		}
	}
	var count int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM deployments WHERE service_id=$1 AND status NOT IN ('active','failed','superseded')`, serviceID).Scan(&count)
	if err != nil {
		return Deployment{}, err
	}
	var actions int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM service_actions WHERE service_id=$1 AND status IN ('queued','running')`, serviceID).Scan(&actions); err != nil {
		return Deployment{}, err
	}
	if actions > 0 {
		return Deployment{}, errors.New("service action is still running")
	}
	if count >= 10 {
		return Deployment{}, errors.New("deployment queue is full (10 pending per service)")
	}
	d, err := scanDep(tx.QueryRow(ctx, `INSERT INTO deployments(id,service_id,image,port,health_path,settings) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+depColumns, ID(), serviceID, spec.Image, spec.Port, spec.HealthPath, settings))
	if err != nil {
		return d, err
	}
	if key != "" {
		if _, err = tx.Exec(ctx, `UPDATE deployments SET request_key=$2,request_hash=$3 WHERE id=$1`, d.ID, key, hash); err != nil {
			return d, err
		}
	}
	vars, err := s.variables(ctx, tx, serviceID)
	if err != nil {
		return d, err
	}
	plain, err := json.Marshal(vars)
	if err != nil {
		return d, err
	}
	if s.Cipher == nil {
		return d, errors.New("encryption key unavailable")
	}
	encrypted, err := s.Cipher.Seal(string(plain), "deployment:"+d.ID)
	if err != nil {
		return d, err
	}
	if _, err = tx.Exec(ctx, `UPDATE deployments SET encrypted_env=$2 WHERE id=$1`, d.ID, encrypted); err != nil {
		return d, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,stage,message) VALUES($1,'queued','Deployment queued')`, d.ID)
	if err != nil {
		return d, err
	}
	return d, nil
}

func (s *Store) Claim(ctx context.Context) (*Work, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Expired work becomes terminal even if the agent was disconnected. Reconciliation restores routes.
	for _, table := range []string{"deployments", "service_actions"} {
		terminal := "('active','failed','superseded')"
		if table == "service_actions" {
			terminal = "('done','failed')"
		}
		if _, err = tx.Exec(ctx, `UPDATE `+table+` SET status='failed',error='Execution retry limit or deadline reached; desired state will be reconciled',updated_at=now() WHERE status NOT IN `+terminal+` AND (deadline<now() OR (attempt>=3 AND next_attempt<=now()))`); err != nil {
			return nil, err
		}
	}
	var action Action
	actionErr := tx.QueryRow(ctx, `SELECT id,service_id,kind,status,error,created_at,backup_id FROM service_actions WHERE status IN ('queued','running') AND next_attempt<=now() ORDER BY created_at,id LIMIT 1 FOR UPDATE`).Scan(&action.ID, &action.ServiceID, &action.Kind, &action.Status, &action.Error, &action.CreatedAt, &action.BackupID)
	if actionErr == nil {
		v, e := scanSvc(tx.QueryRow(ctx, `SELECT id,project_id,name,environment,host,active_id,created_at,desired_state,settings FROM services WHERE id=$1`, action.ServiceID))
		if e != nil {
			return nil, e
		}
		d, e := scanDep(tx.QueryRow(ctx, `SELECT `+depColumns+` FROM deployments WHERE id=$1`, v.ActiveID))
		if e != nil {
			return nil, e
		}
		env, e := s.deploymentEnv(ctx, tx, d.ID)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `UPDATE service_actions SET status='running',updated_at=now() WHERE id=$1`, action.ID); e != nil {
			return nil, e
		}
		attempt, e := nextAttempt(ctx, tx, "service_actions", action.ID)
		if e != nil {
			return nil, e
		}
		return &Work{Attempt: attempt, Action: &action, Service: v, Deployment: d, Env: env}, tx.Commit(ctx)
	}
	if !errors.Is(actionErr, pgx.ErrNoRows) {
		return nil, actionErr
	}
	// The filesystem lock serializes execution; attempt tokens fence stale acknowledgements.
	d, err := scanDep(tx.QueryRow(ctx, `SELECT `+depColumns+` FROM deployments WHERE status NOT IN ('active','failed','superseded') AND next_attempt<=now() AND NOT EXISTS(SELECT 1 FROM deployments earlier WHERE earlier.service_id=deployments.service_id AND earlier.status NOT IN ('active','failed','superseded') AND (earlier.created_at,earlier.id)<(deployments.created_at,deployments.id)) ORDER BY created_at,id LIMIT 1 FOR UPDATE`))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	v, err := scanSvc(tx.QueryRow(ctx, `SELECT id,project_id,name,environment,host,active_id,created_at,desired_state,settings FROM services WHERE id=$1`, d.ServiceID))
	if err != nil {
		return nil, err
	}
	attempt, err := nextAttempt(ctx, tx, "deployments", d.ID)
	if err != nil {
		return nil, err
	}
	w := &Work{Attempt: attempt, Deployment: d, Service: v}
	if v.ActiveID != "" && v.DesiredState != "stopped" {
		p, e := scanDep(tx.QueryRow(ctx, `SELECT `+depColumns+` FROM deployments WHERE id=$1`, v.ActiveID))
		if e != nil {
			return nil, e
		}
		p.Env, e = s.deploymentEnv(ctx, tx, p.ID)
		if e != nil {
			return nil, e
		}
		w.Previous = &p
	}
	w.Env, err = s.deploymentEnv(ctx, tx, d.ID)
	if err != nil {
		return nil, err
	}
	return w, tx.Commit(ctx)
}
func (s *Store) Report(ctx context.Context, id string, r Report) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	d, err := scanDep(tx.QueryRow(ctx, `SELECT `+depColumns+` FROM deployments WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	env, envErr := s.deploymentEnv(ctx, tx, id)
	if envErr != nil {
		return envErr
	}
	for _, value := range env {
		if value != "" {
			r.Logs = strings.ReplaceAll(r.Logs, value, "[REDACTED]")
			r.Message = strings.ReplaceAll(r.Message, value, "[REDACTED]")
		}
	}
	if r.Status == "" {
		_, err = tx.Exec(ctx, `UPDATE deployments SET logs=$2 WHERE id=$1`, id, r.Logs)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err = checkAttempt(ctx, tx, "deployments", id, r.Attempt, r.Status == "active"); err != nil {
		return err
	}
	if Terminal(d.Status) {
		if d.Status == r.Status {
			return nil
		}
		return fmt.Errorf("deployment is already %s", d.Status)
	}
	if r.Status == "active" {
		var active string
		err = tx.QueryRow(ctx, `SELECT active_id FROM services WHERE id=$1 FOR UPDATE`, d.ServiceID).Scan(&active)
		if err != nil {
			return err
		}
		if active != "" {
			_, err = tx.Exec(ctx, `UPDATE deployments SET status='superseded',updated_at=now() WHERE id=$1 AND status='active'`, active)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE services SET active_id=$2,desired_state='running' WHERE id=$1`, d.ServiceID, id)
		if err != nil {
			return err
		}
	}
	failure := ""
	if r.Status == "failed" {
		failure = r.Message
	}
	_, err = tx.Exec(ctx, `UPDATE deployments SET status=$2,error=$3,logs=$4,updated_at=now() WHERE id=$1`, id, r.Status, failure, r.Logs)
	if err != nil {
		return err
	}
	if d.Status != r.Status {
		_, err = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,stage,message) VALUES($1,$2,$3)`, id, r.Status, r.Message)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) State(ctx context.Context) (State, error) {
	state := State{Environments: []Environment{}, Actions: []Action{}, Projects: []Project{}, Services: []Service{}, Deployments: []Deployment{}, Events: []Event{}}
	// One consistent snapshot avoids briefly pairing a new active pointer with old history.
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return state, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,name,created_at FROM projects ORDER BY created_at`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var p Project
		if err = rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			rows.Close()
			return state, err
		}
		state.Projects = append(state.Projects, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = tx.Query(ctx, `SELECT id,project_id,name,environment,host,active_id,created_at,desired_state,settings FROM services ORDER BY created_at`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		v, e := scanSvc(rows)
		if e != nil {
			rows.Close()
			return state, e
		}
		state.Services = append(state.Services, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = tx.Query(ctx, `SELECT `+depColumns+` FROM deployments WHERE id IN (SELECT id FROM deployments ORDER BY created_at DESC LIMIT 200) OR status NOT IN ('failed','superseded') ORDER BY created_at DESC`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		d, e := scanDep(rows)
		if e != nil {
			rows.Close()
			return state, e
		}
		state.Deployments = append(state.Deployments, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = tx.Query(ctx, `SELECT id,deployment_id,stage,message,created_at FROM deployment_events ORDER BY id DESC LIMIT 600`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.ID, &e.DeploymentID, &e.Stage, &e.Message, &e.CreatedAt); err != nil {
			rows.Close()
			return state, err
		}
		state.Events = append(state.Events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = tx.Query(ctx, `SELECT project_id,name FROM environments ORDER BY created_at,name`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var e Environment
		if err = rows.Scan(&e.ProjectID, &e.Name); err != nil {
			rows.Close()
			return state, err
		}
		state.Environments = append(state.Environments, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = tx.Query(ctx, `SELECT id,service_id,kind,status,error,created_at,backup_id FROM service_actions ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var a Action
		if err = rows.Scan(&a.ID, &a.ServiceID, &a.Kind, &a.Status, &a.Error, &a.CreatedAt, &a.BackupID); err != nil {
			rows.Close()
			return state, err
		}
		state.Actions = append(state.Actions, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	return state, tx.Commit(ctx)
}
