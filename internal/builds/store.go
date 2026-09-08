package builds

import (
	"cloudrail/internal/deployment"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

type Store struct {
	DB          *pgxpool.Pool
	Deployments *deployment.Store
}

const columns = `id,service_id,commit_sha,config,status,image,deployment_id,logs,error,created_at`

func scan(row pgx.Row) (Build, error) {
	var b Build
	var raw []byte
	e := row.Scan(&b.ID, &b.ServiceID, &b.Commit, &raw, &b.Status, &b.Image, &b.DeploymentID, &b.Logs, &b.Error, &b.CreatedAt)
	if e == nil {
		e = json.Unmarshal(raw, &b.Config)
	}
	return b, e
}
func (s *Store) Source(ctx context.Context, id string) (Config, error) {
	var c Config
	var b []byte
	e := s.DB.QueryRow(ctx, `SELECT config FROM service_sources WHERE service_id=$1 AND source_type='github'`, id).Scan(&b)
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	return c, e
}
func (s *Store) Save(ctx context.Context, id string, c Config) error {
	if e := c.Validate(); e != nil {
		return e
	}
	b, _ := json.Marshal(c)
	_, e := s.DB.Exec(ctx, `INSERT INTO service_sources(service_id,config,source_type) VALUES($1,$2,'github') ON CONFLICT(service_id) DO UPDATE SET config=$2,source_type='github',updated_at=now()`, id, b)
	return e
}
func enqueue(ctx context.Context, tx pgx.Tx, service, sha, key string, c Config) (Build, error) {
	if !ValidSHA(sha) || len(key) > 150 {
		return Build{}, errors.New("invalid build commit or request key")
	}
	raw, _ := json.Marshal(c)
	// Serialize queue limits and request retries per service.
	var id string
	if e := tx.QueryRow(ctx, `SELECT id FROM services WHERE id=$1 FOR UPDATE`, service).Scan(&id); e != nil {
		return Build{}, e
	}
	if key != "" {
		b, e := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM source_builds WHERE service_id=$1 AND request_key=$2`, service, key))
		if e == nil {
			if b.Commit != sha || b.Config != c {
				return b, deployment.ErrConflict
			}
			return b, nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return b, e
		}
	}
	var count int
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM source_builds WHERE service_id=$1 AND status IN ('queued','building')`, service).Scan(&count); e != nil {
		return Build{}, e
	}
	if count >= 10 {
		return Build{}, errors.New("build queue is full")
	}
	var k any
	if key != "" {
		k = key
	}
	return scan(tx.QueryRow(ctx, `INSERT INTO source_builds(id,service_id,commit_sha,config,request_key) VALUES($1,$2,$3,$4,$5) RETURNING `+columns, deployment.ID(), service, sha, raw, k))
}
func (s *Store) Enqueue(ctx context.Context, service, sha, key string, c Config) (Build, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Build{}, e
	}
	defer tx.Rollback(ctx)
	b, e := enqueue(ctx, tx, service, sha, key, c)
	if e != nil {
		return b, e
	}
	return b, tx.Commit(ctx)
}
func (s *Store) Push(ctx context.Context, delivery, repo, branch, sha string, installation int64) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, `INSERT INTO github_deliveries(id) VALUES($1) ON CONFLICT DO NOTHING`, delivery)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	rows, e := tx.Query(ctx, `SELECT service_id,config FROM service_sources WHERE config->>'repository'=$1 AND config->>'branch'=$2 AND config->>'installation'=$3 AND config->>'autoDeploy'='true' ORDER BY service_id`, repo, branch, jsonNumber(installation))
	if e != nil {
		return e
	}
	type item struct {
		id string
		c  Config
	}
	items := []item{}
	for rows.Next() {
		var i item
		var raw []byte
		if e = rows.Scan(&i.id, &raw); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(raw, &i.c); e != nil {
			rows.Close()
			return e
		}
		items = append(items, i)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, i := range items {
		if _, e = enqueue(ctx, tx, i.id, sha, "webhook:"+delivery, i.c); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func jsonNumber(v int64) string { b, _ := json.Marshal(v); return string(b) }
func (s *Store) Get(ctx context.Context, id string) (Build, error) {
	return scan(s.DB.QueryRow(ctx, `SELECT `+columns+` FROM source_builds WHERE id=$1`, id))
}
func (s *Store) List(ctx context.Context, service string) ([]Build, error) {
	rows, e := s.DB.Query(ctx, `SELECT `+columns+` FROM source_builds WHERE service_id=$1 ORDER BY created_at DESC LIMIT 30`, service)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Build{}
	for rows.Next() {
		b, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Store) Claim(ctx context.Context) (*Build, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE source_builds SET status=CASE WHEN cancel_requested THEN 'cancelled' ELSE 'failed' END,error='Build execution deadline or retry limit reached',updated_at=now() WHERE status IN ('queued','building') AND (deadline<now() OR (attempt>=3 AND next_attempt<=now()))`); e != nil {
		return nil, e
	}
	b, e := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM source_builds WHERE status IN ('queued','building') AND next_attempt<=now() ORDER BY created_at,id LIMIT 1 FOR UPDATE`))
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if e != nil {
		return nil, e
	}
	b.Attempt = deployment.ID()
	_, e = tx.Exec(ctx, `UPDATE source_builds SET status='building',attempt=attempt+1,attempt_token=$2,deadline=COALESCE(deadline,now()+interval '45 minutes'),next_attempt=now()+make_interval(secs=>5*power(2,attempt)::int),updated_at=now() WHERE id=$1`, b.ID, b.Attempt)
	if e != nil {
		return nil, e
	}
	return &b, tx.Commit(ctx)
}
func (s *Store) Report(ctx context.Context, id, attempt, status, image, logs, message string) error {
	if len(logs) > 16000 || len(message) > 1800 {
		return errors.New("build report too large")
	}
	if status != "building" && status != "succeeded" && status != "failed" && status != "cancelled" {
		return errors.New("invalid build status")
	}
	if status == "succeeded" {
		if !(strings.HasPrefix(image, "127.0.0.1:5001/cloudrail/") && (deployment.Spec{Image: image, Port: 80, HealthPath: "/"}).Validate() == nil) {
			return errors.New("invalid build image digest")
		}
	}
	tag, e := s.DB.Exec(ctx, `UPDATE source_builds SET status=CASE WHEN cancel_requested AND $3<>'building' THEN 'cancelled' ELSE $3 END,image=$4,logs=$5,error=$6,updated_at=now() WHERE id=$1 AND attempt_token=$2 AND $2<>'' AND deadline>now() AND status IN ('queued','building',$3) AND NOT (cancel_requested AND $3='succeeded')`, id, attempt, status, image, logs, message)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return deployment.ErrConflict
	}
	return nil
}
func (s *Store) Cancel(ctx context.Context, id string) error {
	tag, e := s.DB.Exec(ctx, `UPDATE source_builds SET cancel_requested=true,status=CASE WHEN status='queued' THEN 'cancelled' ELSE status END WHERE id=$1 AND status IN ('queued','building')`, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return deployment.ErrConflict
	}
	return nil
}

// A successful build is durable before enqueue. The deployment idempotency key recovers a crash between these commits.
func (s *Store) Flush(ctx context.Context) error {
	rows, e := s.DB.Query(ctx, `SELECT `+columns+` FROM source_builds WHERE status='succeeded' AND deployment_id='' ORDER BY created_at LIMIT 10`)
	if e != nil {
		return e
	}
	items := []Build{}
	for rows.Next() {
		b, e := scan(rows)
		if e != nil {
			rows.Close()
			return e
		}
		items = append(items, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, b := range items {
		d, e := s.Deployments.Enqueue(ctx, b.ServiceID, deployment.Spec{Image: b.Image, Port: b.Config.Port, HealthPath: b.Config.HealthPath}, "build:"+b.ID)
		if e != nil {
			return e
		}
		if _, e = s.DB.Exec(ctx, `UPDATE source_builds SET deployment_id=$2 WHERE id=$1 AND deployment_id=''`, b.ID, d.ID); e != nil {
			return e
		}
	}
	return nil
}
