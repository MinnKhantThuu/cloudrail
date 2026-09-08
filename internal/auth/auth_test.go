package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCookieAndOriginBoundary(t *testing.T) {
	a := &Auth{Secure: true}
	w := httptest.NewRecorder()
	a.Cookie(w, "token")
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 86400 {
		t.Fatal("unsafe session cookie")
	}
	r := httptest.NewRequest("POST", "https://control.example/api/projects", nil)
	if SameOrigin(r) {
		t.Fatal("write without CSRF header accepted")
	}
	r.Header.Set("X-Cloudrail-Request", "1")
	r.Header.Set("Origin", "https://evil.example")
	if SameOrigin(r) {
		t.Fatal("cross-origin write accepted")
	}
	r.Header.Set("Origin", "https://control.example")
	if !SameOrigin(r) {
		t.Fatal("same-origin rejected")
	}
}
