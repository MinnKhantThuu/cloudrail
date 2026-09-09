package deployment

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"

	"cloudrail/internal/secrets"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTypedVariableReferenceLifecycle(t *testing.T) {
	if os.Getenv("CLOUDRAIL_INTEGRATION") != "1" {
		t.Skip("requires PostgreSQL integration environment")
	}
	ctx := context.Background()
	base := os.Getenv("DATABASE_URL")
	db, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := "variable_verify_" + ID()
	if _, err = db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
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

	project, err := store.CreateProject(ctx, "typed variables")
	if err != nil {
		t.Fatal(err)
	}
	backend, err := store.CreateService(ctx, project.ID, "backend")
	if err != nil {
		t.Fatal(err)
	}
	frontend, err := store.CreateService(ctx, project.ID, "frontend")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateEnvironment(ctx, project.ID, "staging"); err != nil {
		t.Fatal(err)
	}
	staging, err := store.CreateService(ctx, project.ID, "staging-api", "staging")
	if err != nil {
		t.Fatal(err)
	}

	if err = store.SetVariableTyped(ctx, backend.ID, "API_ORIGIN", "http://backend.internal", "plain"); err != nil {
		t.Fatal(err)
	}
	if err = store.SetVariableTyped(ctx, frontend.ID, "SESSION_SECRET", "never-return-this", "secret"); err != nil {
		t.Fatal(err)
	}
	if err = store.SetVariableTyped(ctx, frontend.ID, "PUBLIC_LABEL", "storefront", "plain"); err != nil {
		t.Fatal(err)
	}
	if err = store.SetReference(ctx, frontend.ID, "API_URL", backend.ID, "API_ORIGIN"); err != nil {
		t.Fatal(err)
	}

	items, err := store.Variables(ctx, frontend.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(items)
	contract := string(raw)
	if strings.Contains(contract, "never-return-this") || !strings.Contains(contract, `"kind":"secret"`) || !strings.Contains(contract, `"value":"storefront"`) || !strings.Contains(contract, `"targetServiceName":"backend"`) {
		t.Fatalf("unsafe or incomplete variable metadata: %s", contract)
	}
	if err = store.SetReference(ctx, frontend.ID, "CROSS_ENV", staging.ID, "ANYTHING"); err == nil || !strings.Contains(err.Error(), "same project and environment") {
		t.Fatalf("cross-environment reference was not rejected: %v", err)
	}
	if err = store.SetReference(ctx, backend.ID, "LOOP", frontend.ID, "API_URL"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("reference cycle was not rejected: %v", err)
	}

	spec := Spec{Image: "nginx@sha256:" + strings.Repeat("a", 64), Port: 80, HealthPath: "/"}
	first, err := store.Enqueue(ctx, frontend.ID, spec, "reference-first")
	if err != nil {
		t.Fatal(err)
	}
	env, err := store.deploymentEnv(ctx, store.DB, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if env["API_URL"] != "http://backend.internal" || env["SESSION_SECRET"] != "never-return-this" {
		t.Fatalf("first deployment did not resolve variables: %#v", env)
	}
	if _, err = store.DB.Exec(ctx, `UPDATE deployments SET status='failed' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.SetVariableTyped(ctx, backend.ID, "API_ORIGIN", "http://backend-v2.internal", "plain"); err != nil {
		t.Fatal(err)
	}
	second, err := store.Enqueue(ctx, frontend.ID, spec, "reference-second")
	if err != nil {
		t.Fatal(err)
	}
	env, err = store.deploymentEnv(ctx, store.DB, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if env["API_URL"] != "http://backend-v2.internal" {
		t.Fatalf("redeployment kept a stale reference: %#v", env)
	}
	if err = store.RenameVariable(ctx, backend.ID, "API_ORIGIN", "INTERNAL_ORIGIN"); err != nil {
		t.Fatal(err)
	}
	items, err = store.Variables(ctx, frontend.ID)
	if err != nil || len(items) != 3 || items[0].Name != "API_URL" || items[0].TargetVariable != "INTERNAL_ORIGIN" {
		t.Fatalf("target rename did not update the reference: %#v %v", items, err)
	}
	if err = store.DeleteVariable(ctx, backend.ID, "INTERNAL_ORIGIN"); err == nil || !strings.Contains(err.Error(), "referenced") {
		t.Fatalf("referenced target deletion was not guarded: %v", err)
	}
	if err = store.DeleteVariable(ctx, frontend.ID, "API_URL"); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteVariable(ctx, backend.ID, "INTERNAL_ORIGIN"); err != nil {
		t.Fatal(err)
	}
}
