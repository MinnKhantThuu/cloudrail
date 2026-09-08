package deployment

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"cloudrail/internal/secrets"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCronClaimOverlapHistoryAndRecovery(t *testing.T) {
	if os.Getenv("CLOUDRAIL_INTEGRATION") != "1" {
		t.Skip("requires PostgreSQL integration environment")
	}
	ctx := context.Background()
	base := os.Getenv("DATABASE_URL")
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "cron_verify_" + ID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	store, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	store.Cipher, err = secrets.New(os.Getenv("CLOUDRAIL_ENCRYPTION_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(ctx, "cron integration")
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateCompute(ctx, project.ID, ComputeSpec{Name: "cleanup", Environment: "production", WorkloadMode: "cron", SourceType: "image"})
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{Image: "example@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Port: 80, HealthPath: "/"}
	if _, err = store.Enqueue(ctx, service.ID, spec); err == nil {
		t.Fatal("cron deployment accepted before schedule")
	}
	service, err = store.SaveCronSchedule(ctx, service.ID, "*/5 * * * *")
	if err != nil || service.CronNextRun == nil {
		t.Fatalf("save schedule: %#v %v", service, err)
	}
	deployment, err := store.Enqueue(ctx, service.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE deployments SET status='active' WHERE id=$1`, deployment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE services SET active_id=$1,cron_next_run=now()-interval '1 minute' WHERE id=$2`, deployment.ID, service.ID); err != nil {
		t.Fatal(err)
	}
	work, err := store.Claim(ctx)
	if err != nil || work == nil || work.CronRun == nil || work.CronRun.ServiceID != service.ID {
		t.Fatalf("claim due run: %#v %v", work, err)
	}
	firstID := work.CronRun.ID
	if _, err = store.DB.Exec(ctx, `UPDATE cron_runs SET next_attempt=now()-interval '1 second' WHERE id=$1`, firstID); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Claim(ctx)
	if err != nil || recovered == nil || recovered.CronRun == nil || recovered.CronRun.ID != firstID || recovered.Attempt == work.Attempt {
		t.Fatalf("unfinished run was not recovered: %#v %v", recovered, err)
	}
	exit := 0
	if err = store.ReportCronRun(ctx, firstID, Report{Attempt: recovered.Attempt, Status: "succeeded", ExitCode: &exit, Logs: "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE services SET cron_next_run=now()-interval '1 minute' WHERE id=$1`, service.ID); err != nil {
		t.Fatal(err)
	}
	next, err := store.Claim(ctx)
	if err != nil || next == nil || next.CronRun == nil || next.CronRun.ID == firstID {
		t.Fatalf("next occurrence missing: %#v %v", next, err)
	}
	state, err := store.State(ctx)
	if err != nil || len(state.CronRuns) != 2 || state.CronRuns[1].Status != "succeeded" || state.CronRuns[1].FinishedAt == nil {
		t.Fatalf("cron history missing: %#v %v", state.CronRuns, err)
	}
	if next.Service.CronNextRun == nil || !next.Service.CronNextRun.After(time.Now().Add(-time.Second)) {
		t.Fatalf("missed schedules were not coalesced: %#v", next.Service.CronNextRun)
	}
}
