package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentAndAdminTokensAreNotInterchangeable(t *testing.T) {
	admin := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	agent := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hit := false
	handler := Auth(agent, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true; w.WriteHeader(204) }))
	for _, token := range []string{"", admin, agent} {
		hit = false
		r := httptest.NewRequest("POST", "/internal/claim", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if token == agent {
			if !hit || w.Code != 204 {
				t.Fatal("agent rejected")
			}
		} else if hit || w.Code != 401 {
			t.Fatal("unauthorized caller allowed")
		}
	}
}
