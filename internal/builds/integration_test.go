package builds_test

import (
	"bytes"
	"cloudrail/internal/api"
	"cloudrail/internal/builds"
	"cloudrail/internal/deployment"
	"cloudrail/internal/githubapp"
	"cloudrail/internal/secrets"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestPostgresSignedPush(t *testing.T) {
	if os.Getenv("CLOUDRAIL_INTEGRATION") != "1" {
		t.Skip("requires integration DB")
	}
	ctx := context.Background()
	base := os.Getenv("DATABASE_URL")
	db, e := pgxpool.New(ctx, base)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	schema := "verify_" + deployment.ID()
	if _, e = db.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, e := deployment.Open(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer store.DB.Close()
	store.Cipher, e = secrets.New(os.Getenv("CLOUDRAIL_ENCRYPTION_KEY"))
	if e != nil {
		t.Fatal(e)
	}
	gh := githubapp.New(store.DB, store.Cipher)
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	secret := "test-webhook-secret-never-used-for-real-github"
	e = gh.Save(ctx, githubapp.Configuration{AppID: "123", Slug: "test", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), WebhookSecret: secret})
	if e != nil {
		t.Fatal(e)
	}
	bstore := &builds.Store{DB: store.DB, Deployments: store}
	p, e := store.CreateProject(ctx, "webhook test")
	if e != nil {
		t.Fatal(e)
	}
	s, e := store.CreateService(ctx, p.ID, "api")
	if e != nil {
		t.Fatal(e)
	}
	config := builds.Config{Repository: "owner/repo", Branch: "main", Installation: 42, Builder: "dockerfile", Port: 80, HealthPath: "/", AutoDeploy: true}
	if e = bstore.Save(ctx, s.ID, config); e != nil {
		t.Fatal(e)
	}
	a := &api.API{Store: store, Builds: bstore, GitHub: gh}
	handler := a.Handler(t.TempDir())
	payload, _ := json.Marshal(map[string]any{"ref": "refs/heads/main", "after": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "repository": map[string]string{"full_name": "owner/repo"}, "installation": map[string]int{"id": 42}})
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(payload)
	signature := "sha256=" + hex.EncodeToString(m.Sum(nil))
	deliver := func(body []byte, sig string) int {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", sig)
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-GitHub-Delivery", "unique-delivery-1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}
	if deliver(payload, "invalid") != 401 {
		t.Fatal("unsigned push accepted")
	}
	if deliver(payload, signature) != 202 || deliver(payload, signature) != 202 {
		t.Fatal("signed push/retry failed")
	}
	list, e := bstore.List(ctx, s.ID)
	if e != nil || len(list) != 1 {
		t.Fatalf("delivery was duplicated: %v", e)
	}
	work, e := bstore.Claim(ctx)
	if e != nil || work == nil {
		t.Fatal(e)
	}
	if e = bstore.Report(ctx, work.ID, "stale", "failed", "", "", ""); e != deployment.ErrConflict {
		t.Fatal("stale build report accepted")
	}
	if e = bstore.Cancel(ctx, work.ID); e != nil {
		t.Fatal(e)
	}
	image := "127.0.0.1:5001/cloudrail/" + s.ID + "@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if e = bstore.Report(ctx, work.ID, work.Attempt, "succeeded", image, "", ""); e != deployment.ErrConflict {
		t.Fatal("cancelled build published")
	}
	if e = bstore.Report(ctx, work.ID, work.Attempt, "cancelled", "", "", ""); e != nil {
		t.Fatal(e)
	}
	retry, e := bstore.Enqueue(ctx, s.ID, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "retry-test", config)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		_, e = store.DB.Exec(ctx, `UPDATE source_builds SET next_attempt=now()-interval '1 second' WHERE id=$1`, retry.ID)
		if e != nil {
			t.Fatal(e)
		}
		work, e = bstore.Claim(ctx)
		if e != nil || work == nil {
			t.Fatalf("build retry %d: %v", i, e)
		}
	}
	_, e = store.DB.Exec(ctx, `UPDATE source_builds SET next_attempt=now()-interval '1 second' WHERE id=$1`, retry.ID)
	if e != nil {
		t.Fatal(e)
	}
	work, e = bstore.Claim(ctx)
	if e != nil || work != nil {
		t.Fatal("exhausted build reclaimed")
	}
	finished, e := bstore.Get(ctx, retry.ID)
	if e != nil || finished.Status != "failed" {
		t.Fatal("build retry limit not persisted")
	}

}
