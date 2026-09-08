package api

import (
	"cloudrail/internal/builds"
	"cloudrail/internal/githubapp"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (a *API) buildRoutes(public, admin, agent *http.ServeMux) {
	admin.HandleFunc("GET /api/github", func(w http.ResponseWriter, r *http.Request) {
		var id, slug string
		e := a.Store.DB.QueryRow(r.Context(), `SELECT app_id,slug FROM github_app`).Scan(&id, &slug)
		if e != nil && e != pgx.ErrNoRows {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]any{"configured": e == nil, "appId": id, "slug": slug})
	})
	admin.HandleFunc("PUT /api/github", func(w http.ResponseWriter, r *http.Request) {
		var b githubapp.Configuration
		if !decode(w, r, &b) {
			return
		}
		if e := a.GitHub.Save(r.Context(), b); e != nil {
			problem(w, 400, e.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("GET /api/github/{kind}", func(w http.ResponseWriter, r *http.Request) {
		installation, _ := strconv.ParseInt(r.URL.Query().Get("installation"), 10, 64)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		repo := r.URL.Query().Get("repository")
		if r.PathValue("kind") == "branches" && !builds.ValidRepository(repo) {
			problem(w, 400, "Invalid repository")
			return
		}
		out, e := a.GitHub.List(r.Context(), r.PathValue("kind"), installation, repo, page)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		write(w, 200, out)
	})
	admin.HandleFunc("GET /api/services/{id}/source", func(w http.ResponseWriter, r *http.Request) {
		c, e := a.Builds.Source(r.Context(), r.PathValue("id"))
		if e == pgx.ErrNoRows {
			write(w, 200, map[string]any{"source": nil})
			return
		}
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]any{"source": c})
	})
	admin.HandleFunc("PUT /api/services/{id}/source", func(w http.ResponseWriter, r *http.Request) {
		settings, e := a.Store.Settings(r.Context(), r.PathValue("id"))
		if e != nil {
			dbError(w, e)
			return
		}
		if settings.Kind != "http" {
			problem(w, 400, "Source builds require an application service")
			return
		}
		var c builds.Config
		if !decode(w, r, &c) {
			return
		}
		if e := c.Validate(); e != nil {
			problem(w, 400, e.Error())
			return
		}
		if _, e := a.GitHub.Commit(r.Context(), c.Installation, c.Repository, c.Branch); e != nil {
			problem(w, 400, e.Error())
			return
		}
		if e := a.Builds.Save(r.Context(), r.PathValue("id"), c); e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("GET /api/services/{id}/builds", func(w http.ResponseWriter, r *http.Request) {
		items, e := a.Builds.List(r.Context(), r.PathValue("id"))
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, items)
	})
	admin.HandleFunc("POST /api/services/{id}/builds", func(w http.ResponseWriter, r *http.Request) {
		c, e := a.Builds.Source(r.Context(), r.PathValue("id"))
		if e != nil {
			dbError(w, e)
			return
		}
		sha, e := a.GitHub.Commit(r.Context(), c.Installation, c.Repository, c.Branch)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		b, e := a.Builds.Enqueue(r.Context(), r.PathValue("id"), sha, r.Header.Get("Idempotency-Key"), c)
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 202, b)
	})
	admin.HandleFunc("POST /api/builds/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if e := a.Builds.Cancel(r.Context(), r.PathValue("id")); e != nil {
			dbError(w, e)
			return
		}
		write(w, 202, map[string]bool{"ok": true})
	})
	public.HandleFunc("POST /webhooks/github", func(w http.ResponseWriter, r *http.Request) {
		raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if e != nil {
			problem(w, 413, "Webhook too large")
			return
		}
		if !a.GitHub.VerifyWebhook(r.Context(), raw, r.Header.Get("X-Hub-Signature-256")) {
			problem(w, 401, "Invalid webhook signature")
			return
		}
		if r.Header.Get("X-GitHub-Event") != "push" {
			write(w, 200, map[string]bool{"ignored": true})
			return
		}
		delivery := r.Header.Get("X-GitHub-Delivery")
		if len(delivery) < 8 || len(delivery) > 128 {
			problem(w, 400, "Delivery ID required")
			return
		}
		var b struct {
			Ref        string `json:"ref"`
			After      string `json:"after"`
			Deleted    bool   `json:"deleted"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
			Installation struct {
				ID int64 `json:"id"`
			} `json:"installation"`
		}
		if json.Unmarshal(raw, &b) != nil {
			problem(w, 400, "Invalid push event")
			return
		}
		if b.Deleted || !strings.HasPrefix(b.Ref, "refs/heads/") {
			write(w, 200, map[string]bool{"ignored": true})
			return
		}
		if !builds.ValidSHA(b.After) || !builds.ValidRepository(b.Repository.FullName) {
			problem(w, 400, "Invalid push commit")
			return
		}
		if e = a.Builds.Push(r.Context(), delivery, b.Repository.FullName, strings.TrimPrefix(b.Ref, "refs/heads/"), b.After, b.Installation.ID); e != nil {
			dbError(w, e)
			return
		}
		write(w, 202, map[string]bool{"ok": true})
	})
	agent.HandleFunc("POST /internal/builds/claim", func(w http.ResponseWriter, r *http.Request) {
		b, e := a.Builds.Claim(r.Context())
		if e != nil {
			dbError(w, e)
			return
		}
		if b == nil {
			w.WriteHeader(204)
			return
		}
		write(w, 200, b)
	})
	agent.HandleFunc("POST /internal/builds/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Status  string `json:"status"`
			Image   string `json:"image"`
			Logs    string `json:"logs"`
			Message string `json:"message"`
		}
		if !decode(w, r, &b) {
			return
		}
		if e := a.Builds.Report(r.Context(), r.PathValue("id"), r.Header.Get("X-Cloudrail-Attempt"), b.Status, b.Image, b.Logs, b.Message); e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	agent.HandleFunc("GET /internal/builds/{id}/cancelled", func(w http.ResponseWriter, r *http.Request) {
		var flag bool
		e := a.Store.DB.QueryRow(r.Context(), `SELECT cancel_requested FROM source_builds WHERE id=$1`, r.PathValue("id")).Scan(&flag)
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]bool{"cancelled": flag})
	})
	agent.HandleFunc("GET /internal/builds/{id}/archive", func(w http.ResponseWriter, r *http.Request) {
		b, e := a.Builds.Get(r.Context(), r.PathValue("id"))
		if e != nil {
			dbError(w, e)
			return
		}
		archive, e := a.GitHub.Archive(r.Context(), b.Config.Installation, b.Config.Repository, b.Commit)
		if e != nil {
			problem(w, 502, e.Error())
			return
		}
		defer archive.Body.Close()
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = io.Copy(w, io.LimitReader(archive.Body, 100<<20))
	})
}
