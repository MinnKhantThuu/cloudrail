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
	rows, err := q.Query(ctx, `SELECT name FROM service_variables WHERE service_id=$1 ORDER BY name`, id)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, name := range names {
		value, resolveErr := s.resolveVariable(ctx, q, id, name, map[string]bool{})
		if resolveErr != nil {
			return nil, resolveErr
		}
		values[name] = value
	}
	return values, nil
}

func (s *Store) resolveVariable(ctx context.Context, q querier, id, name string, visiting map[string]bool) (string, error) {
	key := id + ":" + name
	if visiting[key] {
		return "", errors.New("variable reference cycle")
	}
	visiting[key] = true
	defer delete(visiting, key)
	var ciphertext []byte
	var kind string
	if err := q.QueryRow(ctx, `SELECT ciphertext,kind FROM service_variables WHERE service_id=$1 AND name=$2`, id, name).Scan(&ciphertext, &kind); err != nil {
		return "", err
	}
	if s.Cipher == nil {
		return "", errors.New("encryption key unavailable")
	}
	fallback, err := s.Cipher.Open(ciphertext, "variable:"+id+":"+name)
	if err != nil {
		return "", err
	}
	if kind != "reference" {
		return fallback, nil
	}
	var target, targetVariable string
	err = q.QueryRow(ctx, `SELECT target_service_id,target_variable FROM service_references WHERE source_service_id=$1 AND variable_name=$2`, id, name).Scan(&target, &targetVariable)
	if errors.Is(err, pgx.ErrNoRows) && fallback != "" {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	resolved, err := s.resolveVariable(ctx, q, target, targetVariable, visiting)
	if errors.Is(err, pgx.ErrNoRows) && fallback != "" {
		return fallback, nil
	}
	return resolved, err
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

type Variable struct {
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	Value             string `json:"value,omitempty"`
	TargetServiceID   string `json:"targetServiceId,omitempty"`
	TargetServiceName string `json:"targetServiceName,omitempty"`
	TargetVariable    string `json:"targetVariable,omitempty"`
}

func (s *Store) Variables(ctx context.Context, id string) ([]Variable, error) {
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM services WHERE id=$1)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, pgx.ErrNoRows
	}
	items := []Variable{}
	rows, err := s.DB.Query(ctx, `SELECT v.name,v.kind,v.ciphertext,COALESCE(r.target_service_id,''),COALESCE(t.name,''),COALESCE(r.target_variable,'')
	 FROM service_variables v LEFT JOIN service_references r ON r.source_service_id=v.service_id AND r.variable_name=v.name
	 LEFT JOIN services t ON t.id=r.target_service_id WHERE v.service_id=$1 ORDER BY v.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Variable
		var ciphertext []byte
		if err = rows.Scan(&item.Name, &item.Kind, &ciphertext, &item.TargetServiceID, &item.TargetServiceName, &item.TargetVariable); err != nil {
			return nil, err
		}
		if item.Kind == "plain" {
			if s.Cipher == nil {
				return nil, errors.New("encryption key unavailable")
			}
			item.Value, err = s.Cipher.Open(ciphertext, "variable:"+id+":"+item.Name)
			if err != nil {
				return nil, err
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) VariableNames(ctx context.Context, id string) ([]string, error) {
	items, err := s.Variables(ctx, id)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(items))
	for index, item := range items {
		names[index] = item.Name
	}
	return names, nil
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
	return s.SetVariableTyped(ctx, id, name, value, "secret")
}

func (s *Store) SetVariableTyped(ctx context.Context, id, name, value, kind string) error {
	config, e := s.Settings(ctx, id)
	if e != nil {
		return e
	}
	if IsDataKind(config.Kind) {
		return errors.New("database template credentials are managed by Cloudrail")
	}
	if !variableName.MatchString(name) || len(value) > 8192 || (kind != "secret" && kind != "plain") {
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
	_, err = tx.Exec(ctx, `INSERT INTO service_variables(service_id,name,ciphertext,kind) VALUES($1,$2,$3,$4) ON CONFLICT(service_id,name) DO UPDATE SET ciphertext=$3,kind=$4,updated_at=now()`, id, name, b, kind)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM service_references WHERE source_service_id=$1 AND variable_name=$2`, id, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SetReference(ctx context.Context, source, name, target, targetVariable string) error {
	if !variableName.MatchString(name) || !variableName.MatchString(targetVariable) || source == target {
		return errors.New("invalid variable reference")
	}
	if managed, err := bucketManagedVariable(ctx, s.DB, source, name); err != nil {
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
	var sameScope, targetExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM services s JOIN services t ON t.project_id=s.project_id AND t.environment=s.environment
	 WHERE s.id=$1 AND t.id=$2 AND s.settings->>'kind'='http')`, source, target).Scan(&sameScope); err != nil {
		return err
	}
	if !sameScope {
		return errors.New("references must connect services in the same project and environment")
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM service_variables WHERE service_id=$1 AND name=$2)`, target, targetVariable).Scan(&targetExists); err != nil {
		return err
	}
	if !targetExists {
		return errors.New("target variable not found")
	}
	var cycle bool
	if err = tx.QueryRow(ctx, `WITH RECURSIVE reachable(id) AS (
	 SELECT target_service_id FROM service_references WHERE source_service_id=$1
	 UNION SELECT r.target_service_id FROM service_references r JOIN reachable p ON r.source_service_id=p.id
	) SELECT EXISTS(SELECT 1 FROM reachable WHERE id=$2)`, target, source).Scan(&cycle); err != nil {
		return err
	}
	if cycle {
		return errors.New("variable reference would create a cycle")
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM service_variables WHERE service_id=$1 AND name<>$2`, source, name).Scan(&count); err != nil {
		return err
	}
	if count >= 64 {
		return errors.New("maximum 64 variables per service")
	}
	placeholder, err := s.Cipher.Seal("", "variable:"+source+":"+name)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO service_variables(service_id,name,ciphertext,kind) VALUES($1,$2,$3,'reference')
	 ON CONFLICT(service_id,name) DO UPDATE SET ciphertext=$3,kind='reference',updated_at=now()`, source, name, placeholder); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO service_references(source_service_id,variable_name,target_service_id,target_variable) VALUES($1,$2,$3,$4)
	 ON CONFLICT(source_service_id,variable_name) DO UPDATE SET target_service_id=EXCLUDED.target_service_id,target_variable=EXCLUDED.target_variable,created_at=now()`, source, name, target, targetVariable); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RenameVariable(ctx context.Context, id, oldName, newName string) error {
	if !variableName.MatchString(oldName) || !variableName.MatchString(newName) || oldName == newName {
		return errors.New("choose a different valid variable name")
	}
	config, err := s.Settings(ctx, id)
	if err != nil {
		return err
	}
	if IsDataKind(config.Kind) {
		return errors.New("database template credentials are managed by Cloudrail")
	}
	if s.Cipher == nil {
		return errors.New("encryption key unavailable")
	}
	for _, name := range []string{oldName, newName} {
		if managed, managedErr := bucketManagedVariable(ctx, s.DB, id, name); managedErr != nil {
			return managedErr
		} else if managed {
			return errors.New("bucket connection variables are managed by Cloudrail")
		}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ciphertext []byte
	if err = tx.QueryRow(ctx, `SELECT ciphertext FROM service_variables WHERE service_id=$1 AND name=$2 FOR UPDATE`, id, oldName).Scan(&ciphertext); err != nil {
		return err
	}
	var collision bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM service_variables WHERE service_id=$1 AND name=$2)`, id, newName).Scan(&collision); err != nil {
		return err
	}
	if collision {
		return errors.New("a variable with the new name already exists")
	}
	value, err := s.Cipher.Open(ciphertext, "variable:"+id+":"+oldName)
	if err != nil {
		return err
	}
	ciphertext, err = s.Cipher.Seal(value, "variable:"+id+":"+newName)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE service_variables SET name=$3,ciphertext=$4,updated_at=now() WHERE service_id=$1 AND name=$2`, id, oldName, newName, ciphertext); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE service_references SET variable_name=$3 WHERE source_service_id=$1 AND variable_name=$2`, id, oldName, newName); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE service_references SET target_variable=$3 WHERE target_service_id=$1 AND target_variable=$2`, id, oldName, newName); err != nil {
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
	var referenced bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM service_references WHERE target_service_id=$1 AND target_variable=$2)`, id, name).Scan(&referenced); err != nil {
		return err
	}
	if referenced {
		return errors.New("variable is referenced by another service; remove that reference first")
	}
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
