package api

import (
	"bytes"
	"cloudrail/internal/auth"
	"cloudrail/internal/deployment"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Isolated schema: never reset an installed workspace to exercise first registration.
func TestOwnerRegistration(t *testing.T) {
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
	schema := "owner_verify_" + deployment.ID()
	if _, err = db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, err := deployment.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	a := &API{Sessions: &auth.Auth{DB: s.DB, Secure: true}, Store: s}
	public := http.NewServeMux()
	a.workspaceRoutes(public, http.NewServeMux(), http.NewServeMux())
	request := func(path string, body any, origin string, csrf bool, cookie *http.Cookie) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "https://control.example"+path, bytes.NewReader(raw))
		if csrf {
			r.Header.Set("X-Cloudrail-Request", "1")
		}
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		public.ServeHTTP(w, r)
		return w
	}
	password := "registration-test-password"
	body := map[string]string{"email": "owner@example.com", "password": password, "passwordConfirmation": password}
	for _, v := range []struct {
		origin string
		csrf   bool
	}{{"https://evil.example", true}, {"https://control.example", false}} {
		if w := request("/auth/setup", body, v.origin, v.csrf, nil); w.Code != 403 {
			t.Fatalf("cross-site setup: %d", w.Code)
		}
	}
	bad := map[string]string{"email": "owner@example.com", "password": password, "passwordConfirmation": "different"}
	if w := request("/auth/setup", bad, "", true, nil); w.Code != 400 {
		t.Fatalf("confirmation: %d", w.Code)
	}
	bad["passwordConfirmation"] = "short"
	bad["password"] = "short"
	if w := request("/auth/setup", bad, "", true, nil); w.Code != 400 {
		t.Fatalf("weak password: %d", w.Code)
	}
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); responses <- request("/auth/setup", body, "", true, nil) }()
	}
	wg.Wait()
	close(responses)
	successes, conflicts := 0, 0
	var cookie *http.Cookie
	for w := range responses {
		switch w.Code {
		case 201:
			successes++
			cookie = w.Result().Cookies()[0]
		case 409:
			conflicts++
		default:
			t.Fatalf("setup: %d", w.Code)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent setup: %d success, %d conflict", successes, conflicts)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe session cookie")
	}
	r := httptest.NewRequest("GET", "https://control.example/auth/status", nil)
	r.AddCookie(cookie)
	if email, err := a.Sessions.Session(ctx, r); err != nil || email != body["email"] {
		t.Fatal("setup did not sign in owner")
	}
	if w := request("/auth/setup", body, "", true, nil); w.Code != 409 {
		t.Fatalf("reopened setup: %d", w.Code)
	}
	if w := request("/auth/logout", nil, "", true, cookie); w.Code != 200 {
		t.Fatalf("logout: %d", w.Code)
	}
	if _, err := a.Sessions.Session(ctx, r); err == nil {
		t.Fatal("logout left session valid")
	}
	login := map[string]string{"email": body["email"], "password": "wrong"}
	if w := request("/auth/login", login, "", true, nil); w.Code != 401 {
		t.Fatalf("wrong login: %d", w.Code)
	}
	login["password"] = password
	if w := request("/auth/login", login, "", true, nil); w.Code != 200 {
		t.Fatalf("login: %d", w.Code)
	}
}
