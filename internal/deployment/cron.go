package deployment

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func CronNext(schedule string, after time.Time) (time.Time, error) {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" || strings.HasPrefix(schedule, "@") || len(schedule) > 100 {
		return time.Time{}, errors.New("use a five-field UTC cron schedule such as */15 * * * *")
	}
	parsed, err := cronParser.Parse(schedule)
	if err != nil {
		return time.Time{}, errors.New("use a valid five-field UTC cron schedule")
	}
	return parsed.Next(after.UTC()), nil
}

func (s *Store) SaveCronSchedule(ctx context.Context, id, schedule string) (Service, error) {
	schedule = strings.TrimSpace(schedule)
	next, err := CronNext(schedule, time.Now())
	if err != nil {
		return Service{}, err
	}
	tag, err := s.DB.Exec(ctx, `UPDATE services SET cron_schedule=$2,cron_next_run=$3 WHERE id=$1 AND workload_mode='cron'`, id, schedule, next)
	if err != nil {
		return Service{}, err
	}
	if tag.RowsAffected() != 1 {
		return Service{}, pgx.ErrNoRows
	}
	return scanSvc(s.DB.QueryRow(ctx, `SELECT `+svcColumns+` FROM services WHERE id=$1`, id))
}

const cronColumns = `id,service_id,deployment_id,scheduled_for,status,exit_code,logs,error,started_at,finished_at,created_at,updated_at`

func scanCron(row pgx.Row) (CronRun, error) {
	var run CronRun
	err := row.Scan(&run.ID, &run.ServiceID, &run.DeploymentID, &run.ScheduledFor, &run.Status, &run.ExitCode, &run.Logs, &run.Error, &run.StartedAt, &run.FinishedAt, &run.CreatedAt, &run.UpdatedAt)
	return run, err
}
