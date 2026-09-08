package deployment

import (
	"cloudrail/internal/secrets"
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	"testing"
)

// Uses a dedicated schema; never claims or mutates the installed workspace's queue.
func TestPostgresReliability(t *testing.T) {
	if os.Getenv("CLOUDRAIL_INTEGRATION") != "1" {
		t.Skip("requires PostgreSQL integration environment")
	}
	ctx := context.Background()
	base := os.Getenv("DATABASE_URL")
	db, e := pgxpool.New(ctx, base)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	schema := "verify_" + ID()
	if _, e = db.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, e := url.Parse(base)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, e := Open(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	s.Cipher, e = secrets.New(os.Getenv("CLOUDRAIL_ENCRYPTION_KEY"))
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.CreateProject(ctx, "verify")
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.CreateService(ctx, p.ID, "api")
	if e != nil {
		t.Fatal(e)
	}
	spec := Spec{Image: "example@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Port: 80, HealthPath: "/"}
	d, e := s.Enqueue(ctx, v.ID, spec, "same-request")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Enqueue(ctx, v.ID, spec, "same-request")
	if e != nil || again.ID != d.ID {
		t.Fatalf("dedup failed: %v", e)
	}
	changed := spec
	changed.Port = 81
	if _, e = s.Enqueue(ctx, v.ID, changed, "same-request"); e != ErrConflict {
		t.Fatalf("mismatch accepted: %v", e)
	}
	w, e := s.Claim(ctx)
	if e != nil || w == nil {
		t.Fatalf("claim: %v", e)
	}
	if e = s.Report(ctx, d.ID, Report{Status: "active", Attempt: "obsolete"}); e != ErrConflict {
		t.Fatalf("stale attempt accepted: %v", e)
	}
	if e = s.Cancel(ctx, d.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Report(ctx, d.ID, Report{Status: "active", Attempt: w.Attempt}); e != ErrConflict {
		t.Fatalf("cancelled job activated: %v", e)
	}
	if e = s.Report(ctx, d.ID, Report{Status: "failed", Attempt: w.Attempt, Message: "Cancelled"}); e != nil {
		t.Fatal(e)
	}
	retry, e := s.Enqueue(ctx, v.ID, spec)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		if _, e = s.DB.Exec(ctx, `UPDATE deployments SET next_attempt=now()-interval '1 second' WHERE id=$1`, retry.ID); e != nil {
			t.Fatal(e)
		}
		w, e = s.Claim(ctx)
		if e != nil || w == nil || w.Deployment.ID != retry.ID {
			t.Fatalf("retry %d: %v", i, e)
		}
	}
	if _, e = s.DB.Exec(ctx, `UPDATE deployments SET next_attempt=now()-interval '1 second' WHERE id=$1`, retry.ID); e != nil {
		t.Fatal(e)
	}
	w, e = s.Claim(ctx)
	if e != nil || w != nil {
		t.Fatalf("exhausted job reclaimed: %v", e)
	}
	var status string
	if e = s.DB.QueryRow(ctx, `SELECT status FROM deployments WHERE id=$1`, retry.ID).Scan(&status); e != nil || status != "failed" {
		t.Fatalf("not terminal: %v", e)
	}
	if e = s.SaveSettings(ctx, v.ID, 128, 500, "/data"); e != nil {
		t.Fatal(e)
	}
	volume, e := s.Enqueue(ctx, v.ID, spec)
	if e != nil {
		t.Fatal(e)
	}
	if volume.Settings.MemoryMB != 128 || volume.Settings.VolumeName == "" {
		t.Fatal("deployment did not snapshot settings")
	}
	if e = s.SaveSettings(ctx, v.ID, 256, 1000, "/data"); e != nil {
		t.Fatal(e)
	}
	snapshot, e := scanDep(s.DB.QueryRow(ctx, `SELECT `+depColumns+` FROM deployments WHERE id=$1`, volume.ID))
	if e != nil || snapshot.Settings.MemoryMB != 128 {
		t.Fatal("historical resource snapshot changed")
	}
	if s.SaveSettings(ctx, v.ID, 256, 1000, "/other") == nil {
		t.Fatal("volume moved")
	}
	if _, e = s.CreateDatabase(ctx, p.ID, "invalid-target", "absent-environment"); e == nil {
		t.Fatal("database without environment accepted")
	}
	built, e := s.EnqueueBuilt(ctx, v.ID, spec, "node worker.js")
	if e != nil || built.Settings.StartCommand != "node worker.js" {
		t.Fatalf("built deployment did not snapshot its start command: %#v %v", built.Settings, e)
	}
	var remaining int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM services WHERE name='invalid-target'`).Scan(&remaining); e != nil || remaining != 0 {
		t.Fatal("failed database creation left partial state")
	}

}
