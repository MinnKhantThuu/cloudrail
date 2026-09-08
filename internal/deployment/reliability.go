package deployment

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

var ErrConflict = errors.New("request conflicts with current deployment state")

// nextAttempt is called only by the single process holding the host's execution lock.
func nextAttempt(ctx context.Context, tx pgx.Tx, table, id string) (string, error) {
	if table != "deployments" && table != "service_actions" {
		return "", errors.New("invalid job table")
	}
	token := ID()
	tag, e := tx.Exec(ctx, `UPDATE `+table+` SET attempt=attempt+1,attempt_token=$2,deadline=COALESCE(deadline,now()+interval '10 minutes'),next_attempt=now()+make_interval(secs=>5*power(2,attempt)::int) WHERE id=$1 AND attempt<3 AND (deadline IS NULL OR deadline>now())`, id, token)
	if e != nil {
		return "", e
	}
	if tag.RowsAffected() != 1 {
		return "", ErrConflict
	}
	return token, nil
}
func checkAttempt(ctx context.Context, tx pgx.Tx, table, id, token string, activating bool) error {
	var valid bool
	extra := ""
	if table == "deployments" && activating {
		extra = " AND NOT cancel_requested"
	}
	e := tx.QueryRow(ctx, `SELECT attempt_token=$2 AND $2<>'' AND deadline>now()`+extra+` FROM `+table+` WHERE id=$1`, id, token).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return ErrConflict
	}
	return nil
}
func (s *Store) Cancel(ctx context.Context, id string) error {
	tag, e := s.DB.Exec(ctx, `UPDATE deployments SET cancel_requested=true,status=CASE WHEN status='queued' AND attempt=0 THEN 'failed' ELSE status END,error=CASE WHEN status='queued' AND attempt=0 THEN 'Cancelled before execution' ELSE error END,updated_at=now() WHERE id=$1 AND status NOT IN ('active','failed','superseded')`, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// ReconcileWork returns secrets only to the authenticated execution node.
func (s *Store) ReconcileWork(ctx context.Context) ([]Work, error) {
	state, e := s.State(ctx)
	if e != nil {
		return nil, e
	}
	result := []Work{}
	for _, v := range state.Services {
		if v.ActiveID == "" {
			result = append(result, Work{Service: v})
			continue
		}
		d, e := scanDep(s.DB.QueryRow(ctx, `SELECT `+depColumns+` FROM deployments WHERE id=$1`, v.ActiveID))
		if e != nil {
			return nil, e
		}
		env, e := s.deploymentEnv(ctx, s.DB, d.ID)
		if e != nil {
			return nil, e
		}
		result = append(result, Work{Service: v, Deployment: d, Env: env})
	}
	return result, nil
}
