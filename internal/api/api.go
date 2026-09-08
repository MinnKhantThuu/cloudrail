package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"cloudrail/internal/auth"
	"cloudrail/internal/builds"
	"cloudrail/internal/deployment"
	"cloudrail/internal/githubapp"
	"cloudrail/internal/node"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type API struct {
	Builds     *builds.Store
	GitHub     *githubapp.Client
	Authority  *node.Authority
	Sessions   *auth.Auth
	Store      *deployment.Store
	AgentToken string
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, code int, message string) {
	write(w, code, map[string]string{"error": message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		problem(w, 400, "Invalid JSON request")
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		problem(w, 400, "Expected one JSON object")
		return false
	}
	return true
}
func Auth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(supplied)) != 1 {
			problem(w, 401, "Authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func dbError(w http.ResponseWriter, err error) {
	if errors.Is(err, deployment.ErrConflict) {
		problem(w, 409, err.Error())
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "Resource not found")
		return
	}
	var e *pgconn.PgError
	if errors.As(err, &e) {
		if e.Code == "23503" {
			problem(w, 404, "Parent resource not found")
			return
		}
		if e.Code == "23505" {
			problem(w, 409, "A service with this name already exists")
			return
		}
	}
	if strings.Contains(err.Error(), "queue is full") {
		problem(w, 409, err.Error())
		return
	}
	slog.Error("database request failed", "error", err)
	problem(w, 500, "Request could not be saved. Please retry.")
}
func (a *API) Handler(webDir string) http.Handler {
	public := http.NewServeMux()
	admin := http.NewServeMux()
	agent := http.NewServeMux()
	public.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DB.Ping(r.Context()); err != nil {
			problem(w, 503, "Database unavailable")
			return
		}
		write(w, 200, map[string]string{"status": "ok"})
	})
	state := func(w http.ResponseWriter, r *http.Request) {
		v, err := a.Store.State(r.Context())
		if err != nil {
			dbError(w, err)
			return
		}
		write(w, 200, v)
	}
	admin.HandleFunc("GET /api/state", state)
	admin.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &body) {
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if !deployment.ValidName(body.Name) {
			problem(w, 400, "Use 1–60 letters, numbers, spaces, dots, underscores or hyphens")
			return
		}
		v, err := a.Store.CreateProject(r.Context(), body.Name)
		if err != nil {
			dbError(w, err)
			return
		}
		write(w, 201, v)
	})
	admin.HandleFunc("POST /api/projects/{id}/services", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name        string `json:"name"`
			Environment string `json:"environment"`
		}
		if !decode(w, r, &body) {
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if !deployment.ValidName(body.Name) {
			problem(w, 400, "Use a service name of 1–60 letters, numbers, spaces, dots, underscores or hyphens")
			return
		}
		v, err := a.Store.CreateService(r.Context(), r.PathValue("id"), body.Name, body.Environment)
		if err != nil {
			dbError(w, err)
			return
		}
		write(w, 201, v)
	})
	admin.HandleFunc("POST /api/services/{id}/deployments", func(w http.ResponseWriter, r *http.Request) {
		var spec deployment.Spec
		if !decode(w, r, &spec) {
			return
		}
		if err := spec.Validate(); err != nil {
			problem(w, 400, err.Error())
			return
		}
		v, err := a.Store.Enqueue(r.Context(), r.PathValue("id"), spec, r.Header.Get("Idempotency-Key"))
		if err != nil {
			dbError(w, err)
			return
		}
		write(w, 202, v)
	})
	agent.HandleFunc("GET /internal/state", state)
	agent.HandleFunc("POST /internal/claim", func(w http.ResponseWriter, r *http.Request) {
		job, err := a.Store.Claim(r.Context())
		if err != nil {
			dbError(w, err)
			return
		}
		if job == nil {
			w.WriteHeader(204)
			return
		}
		write(w, 200, job)
	})
	agent.HandleFunc("POST /internal/deployments/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		var report deployment.Report
		if !decode(w, r, &report) {
			return
		}
		switch report.Status {
		case "", "pulling", "starting", "checking", "routing", "active", "failed":
		default:
			problem(w, 400, "Invalid deployment stage")
			return
		}
		if len(report.Message) > 2048 || len(report.Logs) > 16384 {
			problem(w, 400, "Report exceeds size limit")
			return
		}
		report.Attempt = r.Header.Get("X-Cloudrail-Attempt")
		if err := a.Store.Report(r.Context(), r.PathValue("id"), report); err != nil {
			dbError(w, err)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	a.workspaceRoutes(public, admin, agent)
	a.nodeRoutes(public, admin, agent)
	a.buildRoutes(public, admin, agent)
	a.operationRoutes(admin, agent)
	a.reliabilityRoutes(admin, agent)
	public.Handle("/api/", a.ownerOnly(admin))
	public.Handle("/internal/", a.nodeOnly(agent))
	public.Handle("/", http.FileServer(http.Dir(webDir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		public.ServeHTTP(w, r)
	})
}
