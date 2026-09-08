package migrations

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResourceCanvasMigrationPreservesLegacyServices(t *testing.T) {
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
	schema := "migration_verify_0123456789abcdef01234567"
	if _, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE; CREATE SCHEMA "+schema); err != nil {
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
	db, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(ctx, `CREATE TABLE schema_migrations(name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_workspace.sql", "003_reliability.sql", "004_source_builds.sql", "005_operations.sql"} {
		data, readErr := files.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = db.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply legacy %s: %v", name, err)
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(data))
		if _, err = db.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, name, sum); err != nil {
			t.Fatal(err)
		}
	}

	const project = "111111111111111111111111"
	const application = "222222222222222222222222"
	const database = "333333333333333333333333"
	if _, err = db.Exec(ctx, `INSERT INTO projects(id,name) VALUES($1,'legacy')`, project); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO environments(project_id,name) VALUES($1,'production')`, project); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO services(id,project_id,name,environment,host,settings) VALUES
		 ($2,$1,'api','production','api.localhost','{"kind":"http","memoryMB":256,"cpuMillis":1000,"mountPath":"/data","volumeName":"cloudrail-volume-222222222222222222222222","network":"legacy"}'),
		 ($3,$1,'postgres','production','database.localhost','{"kind":"postgres","memoryMB":256,"cpuMillis":1000,"mountPath":"/var/lib/postgresql/data","volumeName":"cloudrail-volume-333333333333333333333333","network":"legacy"}')`, project, application, database); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO service_sources(service_id,config) VALUES($1,'{"repository":"owner/repo"}')`, application); err != nil {
		t.Fatal(err)
	}

	if err = Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM services WHERE project_id=$1`, project).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy services were not preserved: count=%d err=%v", count, err)
	}
	var kind, mode, template string
	if err = db.QueryRow(ctx, `SELECT resource_kind,workload_mode,template_key FROM services WHERE id=$1`, database).Scan(&kind, &mode, &template); err != nil {
		t.Fatal(err)
	}
	if kind != "database" || mode != "web" || template != "postgres" {
		t.Fatalf("legacy database was not classified: %s %s %s", kind, mode, template)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM volumes v JOIN volume_attachments a ON a.volume_id=v.id WHERE v.project_id=$1`, project).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy volumes were not normalized: count=%d err=%v", count, err)
	}
	var source string
	if err = db.QueryRow(ctx, `SELECT source_type FROM service_sources WHERE service_id=$1`, application).Scan(&source); err != nil || source != "github" {
		t.Fatalf("legacy source was not classified: %s err=%v", source, err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO deployments(id,service_id,image,port,health_path,status) VALUES($1,$2,'example@sha256:test',8080,'/','predeploy')`, "444444444444444444444444", application); err != nil {
		t.Fatalf("upgraded deployment status constraint rejected predeploy: %v", err)
	}
}
