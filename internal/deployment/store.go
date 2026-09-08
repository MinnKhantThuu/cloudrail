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
	"time"

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
	return s.CreateCompute(ctx, projectID, ComputeSpec{Name: name, Environment: env, WorkloadMode: "web", SourceType: "empty"})
}
func (s *Store) CreateCompute(ctx context.Context, projectID string, spec ComputeSpec) (Service, error) {
	if err := spec.Validate(); err != nil {
		return Service{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Service{}, err
	}
	defer tx.Rollback(ctx)
	v := Service{ID: ID(), ProjectID: projectID, Name: spec.Name, Environment: spec.Environment, DesiredState: "running", ResourceKind: "service", WorkloadMode: spec.WorkloadMode}
	domain := os.Getenv("APP_DOMAIN")
	if domain == "" {
		domain = "localhost"
	}
	v.Host = v.ID + "." + domain
	v.Settings = Settings{Kind: "http", MemoryMB: 256, CPUMillis: 1000, Network: PrivateNetwork(projectID, spec.Environment)}
	settings, _ := json.Marshal(v.Settings)
	err = tx.QueryRow(ctx, `INSERT INTO services(id,project_id,name,host,environment,settings,resource_kind,workload_mode) VALUES($1,$2,$3,$4,$5,$6,'service',$7) RETURNING created_at`, v.ID, projectID, spec.Name, v.Host, spec.Environment, settings, spec.WorkloadMode).Scan(&v.CreatedAt)
	if err != nil {
		return v, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO service_sources(service_id,config,source_type) VALUES($1,'{}',$2)`, v.ID, spec.SourceType); err != nil {
		return v, err
	}
	if v.WorkloadMode == "web" {
		v.URL = "http://" + v.Host + ":8088"
		if os.Getenv("PUBLIC_MODE") == "true" {
			v.URL = "https://" + v.Host
		}
	}
	return v, tx.Commit(ctx)
}

const depColumns = `id,service_id,image,port,health_path,status,error,logs,created_at,updated_at,settings`
const svcColumns = `id,project_id,name,environment,host,active_id,created_at,desired_state,settings,resource_kind,workload_mode,template_key,cron_schedule,cron_next_run`

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
	err := row.Scan(&v.ID, &v.ProjectID, &v.Name, &v.Environment, &v.Host, &v.ActiveID, &v.CreatedAt, &v.DesiredState, &raw, &v.ResourceKind, &v.WorkloadMode, &v.Template, &v.CronSchedule, &v.CronNextRun)
	if err == nil {
		err = json.Unmarshal(raw, &v.Settings)
	}
	if v.ResourceKind != "database" && v.WorkloadMode == "web" {
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
	var workload, schedule string
	err = tx.QueryRow(ctx, `SELECT id,settings,workload_mode,cron_schedule FROM services WHERE id=$1 FOR UPDATE`, serviceID).Scan(&owner, &settings, &workload, &schedule)
	if err != nil {
		return Deployment{}, err
	}
	var config Settings
	if err = json.Unmarshal(settings, &config); err != nil {
		return Deployment{}, err
	}
	if workload == "cron" && schedule == "" {
		return Deployment{}, ErrCronSchedule
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
	for _, table := range []string{"deployments", "service_actions", "cron_runs"} {
		terminal := "('active','failed','superseded')"
		if table == "service_actions" {
			terminal = "('done','failed')"
		} else if table == "cron_runs" {
			terminal = "('succeeded','failed')"
		}
		if _, err = tx.Exec(ctx, `UPDATE `+table+` SET status='failed',error='Execution retry limit or deadline reached; desired state will be reconciled',updated_at=now() WHERE status NOT IN `+terminal+` AND (deadline<now() OR (attempt>=3 AND next_attempt<=now()))`); err != nil {
			return nil, err
		}
	}
	var action Action
	actionErr := tx.QueryRow(ctx, `SELECT id,service_id,kind,status,error,created_at,backup_id FROM service_actions WHERE status IN ('queued','running') AND next_attempt<=now() ORDER BY created_at,id LIMIT 1 FOR UPDATE`).Scan(&action.ID, &action.ServiceID, &action.Kind, &action.Status, &action.Error, &action.CreatedAt, &action.BackupID)
	if actionErr == nil {
		v, e := scanSvc(tx.QueryRow(ctx, `SELECT `+svcColumns+` FROM services WHERE id=$1`, action.ServiceID))
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
		return s.claimCronTx(ctx, tx)
	}
	if err != nil {
		return nil, err
	}
	v, err := scanSvc(tx.QueryRow(ctx, `SELECT `+svcColumns+` FROM services WHERE id=$1`, d.ServiceID))
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

func (s *Store) claimCronTx(ctx context.Context, tx pgx.Tx) (*Work, error) {
	run, err := scanCron(tx.QueryRow(ctx, `SELECT `+cronColumns+` FROM cron_runs WHERE status IN ('queued','running') AND next_attempt<=now() ORDER BY scheduled_for,id LIMIT 1 FOR UPDATE`))
	if errors.Is(err, pgx.ErrNoRows) {
		var serviceID string
		err = tx.QueryRow(ctx, `SELECT id FROM services s WHERE workload_mode='cron' AND desired_state='running' AND active_id<>'' AND cron_schedule<>'' AND cron_next_run<=now() AND NOT EXISTS(SELECT 1 FROM cron_runs r WHERE r.service_id=s.id AND r.status IN ('queued','running')) ORDER BY cron_next_run,id LIMIT 1 FOR UPDATE OF s SKIP LOCKED`).Scan(&serviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, tx.Commit(ctx)
		}
		if err != nil {
			return nil, err
		}
		service, e := scanSvc(tx.QueryRow(ctx, `SELECT `+svcColumns+` FROM services WHERE id=$1`, serviceID))
		if e != nil {
			return nil, e
		}
		if service.CronNextRun == nil {
			return nil, errors.New("cron service has no next run")
		}
		next, e := CronNext(service.CronSchedule, time.Now())
		if e != nil {
			return nil, e
		}
		run, e = scanCron(tx.QueryRow(ctx, `INSERT INTO cron_runs(id,service_id,deployment_id,scheduled_for) VALUES($1,$2,$3,$4) RETURNING `+cronColumns, ID(), service.ID, service.ActiveID, *service.CronNextRun))
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `UPDATE services SET cron_next_run=$2 WHERE id=$1`, service.ID, next); e != nil {
			return nil, e
		}
		service.CronNextRun = &next
	} else if err != nil {
		return nil, err
	}
	service, err := scanSvc(tx.QueryRow(ctx, `SELECT `+svcColumns+` FROM services WHERE id=$1`, run.ServiceID))
	if err != nil {
		return nil, err
	}
	d, err := scanDep(tx.QueryRow(ctx, `SELECT `+depColumns+` FROM deployments WHERE id=$1`, run.DeploymentID))
	if err != nil {
		return nil, err
	}
	env, err := s.deploymentEnv(ctx, tx, d.ID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE cron_runs SET status='running',started_at=COALESCE(started_at,now()),updated_at=now() WHERE id=$1`, run.ID); err != nil {
		return nil, err
	}
	attempt, err := nextAttempt(ctx, tx, "cron_runs", run.ID)
	if err != nil {
		return nil, err
	}
	run.Status = "running"
	return &Work{Attempt: attempt, CronRun: &run, Deployment: d, Service: service, Env: env}, tx.Commit(ctx)
}

func (s *Store) ReportCronRun(ctx context.Context, id string, r Report) error {
	if r.Status != "succeeded" && r.Status != "failed" {
		return errors.New("invalid cron run status")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	run, err := scanCron(tx.QueryRow(ctx, `SELECT `+cronColumns+` FROM cron_runs WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if run.Status == "succeeded" || run.Status == "failed" {
		if run.Status == r.Status {
			return nil
		}
		return ErrConflict
	}
	if err = checkAttempt(ctx, tx, "cron_runs", id, r.Attempt, false); err != nil {
		return err
	}
	env, err := s.deploymentEnv(ctx, tx, run.DeploymentID)
	if err != nil {
		return err
	}
	for _, value := range env {
		if value != "" {
			r.Logs = strings.ReplaceAll(r.Logs, value, "[REDACTED]")
			r.Message = strings.ReplaceAll(r.Message, value, "[REDACTED]")
		}
	}
	if len(r.Logs) > 16000 {
		r.Logs = r.Logs[len(r.Logs)-16000:]
	}
	if len(r.Message) > 1800 {
		r.Message = r.Message[:1800]
	}
	_, err = tx.Exec(ctx, `UPDATE cron_runs SET status=$2,exit_code=$3,logs=$4,error=$5,finished_at=now(),updated_at=now() WHERE id=$1`, id, r.Status, r.ExitCode, strings.ToValidUTF8(r.Logs, ""), strings.ToValidUTF8(r.Message, ""))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
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
	state := State{Environments: []Environment{}, Actions: []Action{}, Projects: []Project{}, Services: []Service{}, Deployments: []Deployment{}, Events: []Event{}, CronRuns: []CronRun{}}
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
	rows, err = tx.Query(ctx, `SELECT `+svcColumns+` FROM services ORDER BY created_at`)
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
	rows, err = tx.Query(ctx, `SELECT `+cronColumns+` FROM cron_runs ORDER BY scheduled_for DESC LIMIT 200`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		run, e := scanCron(rows)
		if e != nil {
			rows.Close()
			return state, e
		}
		state.CronRuns = append(state.CronRuns, run)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	return state, tx.Commit(ctx)
}
