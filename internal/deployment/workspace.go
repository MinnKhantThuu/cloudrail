package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"regexp"
)

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) variables(ctx context.Context, q querier, id string) (map[string]string, error) {
	values := map[string]string{}
	rows, err := q.Query(ctx, `SELECT name,ciphertext FROM service_variables WHERE service_id=$1 ORDER BY name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var b []byte
		if err = rows.Scan(&name, &b); err != nil {
			return nil, err
		}
		if s.Cipher == nil {
			return nil, errors.New("encryption key unavailable")
		}
		v, e := s.Cipher.Open(b, "variable:"+id+":"+name)
		if e != nil {
			return nil, e
		}
		values[name] = v
	}
	return values, rows.Err()
}
func (s *Store) deploymentEnv(ctx context.Context, q querier, id string) (map[string]string, error) {
	var b []byte
	if err := q.QueryRow(ctx, `SELECT encrypted_env FROM deployments WHERE id=$1`, id).Scan(&b); err != nil {
		return nil, err
	}
	v := map[string]string{}
	if len(b) == 0 {
		return v, nil
	}
	if s.Cipher == nil {
		return nil, errors.New("encryption key unavailable")
	}
	plain, err := s.Cipher.Open(b, "deployment:"+id)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(plain), &v)
	return v, err
}
func (s *Store) VariableNames(ctx context.Context, id string) ([]string, error) {
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM services WHERE id=$1)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, pgx.ErrNoRows
	}
	names := []string{}
	rows, err := s.DB.Query(ctx, `SELECT name FROM service_variables WHERE service_id=$1 ORDER BY name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func bucketManagedVariable(ctx context.Context, q querier, service, name string) (bool, error) {
	var managed bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bucket_bindings WHERE service_id=$1 AND $2=ANY(ARRAY[
	 variable_prefix||'_BUCKET',variable_prefix||'_ENDPOINT',variable_prefix||'_REGION',
	 variable_prefix||'_ACCESS_KEY_ID',variable_prefix||'_SECRET_ACCESS_KEY',variable_prefix||'_FORCE_PATH_STYLE']))`, service, name).Scan(&managed)
	return managed, err
}

func (s *Store) SetVariable(ctx context.Context, id, name, value string) error {
	config, e := s.Settings(ctx, id)
	if e != nil {
		return e
	}
	if IsDataKind(config.Kind) {
		return errors.New("database template credentials are managed by Cloudrail")
	}
	if !variableName.MatchString(name) || len(value) > 8192 {
		return errors.New("invalid variable name or value too long")
	}
	if managed, err := bucketManagedVariable(ctx, s.DB, id, name); err != nil {
		return err
	} else if managed {
		return errors.New("bucket connection variables are managed by Cloudrail")
	}
	if s.Cipher == nil {
		return errors.New("encryption key unavailable")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var service string
	if err = tx.QueryRow(ctx, `SELECT id FROM services WHERE id=$1 FOR UPDATE`, id).Scan(&service); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM service_variables WHERE service_id=$1 AND name<>$2`, id, name).Scan(&count); err != nil {
		return err
	}
	if count >= 64 {
		return errors.New("maximum 64 variables per service")
	}
	b, err := s.Cipher.Seal(value, "variable:"+id+":"+name)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO service_variables(service_id,name,ciphertext) VALUES($1,$2,$3) ON CONFLICT(service_id,name) DO UPDATE SET ciphertext=$3,updated_at=now()`, id, name, b)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM service_references WHERE source_service_id=$1 AND variable_name=$2`, id, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) DeleteVariable(ctx context.Context, id, name string) error {
	if managed, err := bucketManagedVariable(ctx, s.DB, id, name); err != nil {
		return err
	} else if managed {
		return errors.New("disconnect the bucket before removing its variables")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM service_variables WHERE service_id=$1 AND name=$2`, id, name); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM service_references WHERE source_service_id=$1 AND variable_name=$2`, id, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) CreateEnvironment(ctx context.Context, project, name string) (Environment, error) {
	e := Environment{ProjectID: project, Name: name}
	_, err := s.DB.Exec(ctx, `INSERT INTO environments(project_id,name) VALUES($1,$2)`, project, name)
	return e, err
}
func (s *Store) EnqueueAction(ctx context.Context, id, kind string, backup ...string) (Action, error) {
	a := Action{ID: ID(), ServiceID: id, Kind: kind, Status: "queued"}
	if kind != "stop" && kind != "start" && kind != "restart" && kind != "backup" && kind != "restore" {
		return a, errors.New("invalid action")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return a, err
	}
	defer tx.Rollback(ctx)
	if len(backup) > 0 {
		a.BackupID = backup[0]
	}
	var active string
	if err = tx.QueryRow(ctx, `SELECT active_id FROM services WHERE id=$1 FOR UPDATE`, id).Scan(&active); err != nil {
		return a, err
	}
	if active == "" {
		return a, errors.New("service has no deployed release")
	}
	if kind == "start" || kind == "restart" {
		var volumeChanged bool
		if err = tx.QueryRow(ctx, `SELECT COALESCE(s.settings->>'volumeName','')<>COALESCE(d.settings->>'volumeName','') OR COALESCE(s.settings->>'mountPath','')<>COALESCE(d.settings->>'mountPath','') FROM services s JOIN deployments d ON d.id=s.active_id WHERE s.id=$1`, id).Scan(&volumeChanged); err != nil {
			return a, err
		}
		if volumeChanged {
			return a, errors.New("volume configuration changed; deploy a new release before starting")
		}
	}
	if kind == "backup" || kind == "restore" {
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT (settings->>'kind' IN ('postgres','mysql','mongo') AND desired_state='running') OR (settings->>'kind' IN ('http','redis') AND settings->>'volumeName'<>'' AND desired_state='stopped') FROM services WHERE id=$1`, id).Scan(&valid); err != nil {
			return a, err
		}
		if !valid {
			return a, errors.New("backup/restore require running PostgreSQL/MySQL/MongoDB or a stopped Redis/application volume")
		}
	}
	if kind == "restore" {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM backups WHERE id=$1)`, a.BackupID).Scan(&exists); err != nil {
			return a, err
		}
		if !exists {
			return a, errors.New("backup not found")
		}
	}
	var busy bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE service_id=$1 AND status NOT IN ('active','failed','superseded')) OR EXISTS(SELECT 1 FROM service_actions WHERE service_id=$1 AND status IN ('queued','running')) OR EXISTS(SELECT 1 FROM cron_runs WHERE service_id=$1 AND status IN ('queued','running'))`, id).Scan(&busy)
	if err != nil {
		return a, err
	}
	if busy {
		return a, errors.New("service has unfinished work")
	}
	err = tx.QueryRow(ctx, `INSERT INTO service_actions(id,service_id,kind,backup_id) VALUES($1,$2,$3,$4) RETURNING created_at`, a.ID, id, kind, a.BackupID).Scan(&a.CreatedAt)
	if err != nil {
		return a, err
	}
	return a, tx.Commit(ctx)
}
func (s *Store) ReportAction(ctx context.Context, id string, r Report) error {
	if r.Status != "done" && r.Status != "failed" {
		return errors.New("invalid action status")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var kind, service, status string
	err = tx.QueryRow(ctx, `SELECT kind,service_id,status FROM service_actions WHERE id=$1 FOR UPDATE`, id).Scan(&kind, &service, &status)
	if err != nil {
		return err
	}
	if err = checkAttempt(ctx, tx, "service_actions", id, r.Attempt, false); err != nil {
		return err
	}
	if status == "done" || status == "failed" {
		if status == r.Status {
			return nil
		}
		return errors.New("action already finished")
	}
	_, err = tx.Exec(ctx, `UPDATE service_actions SET status=$2,error=$3,updated_at=now() WHERE id=$1`, id, r.Status, r.Message)
	if err != nil {
		return err
	}
	if r.Status == "done" && (kind == "start" || kind == "stop" || kind == "restart") {
		desired := "running"
		if kind == "stop" {
			desired = "stopped"
		}
		_, err = tx.Exec(ctx, `UPDATE services SET desired_state=$2 WHERE id=$1`, service, desired)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
