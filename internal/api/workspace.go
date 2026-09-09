package api

import (
	"cloudrail/internal/auth"
	"cloudrail/internal/deployment"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (a *API) ownerOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.SameOrigin(r) {
			problem(w, 403, "Same-origin request required")
			return
		}
		if _, err := a.Sessions.Session(r.Context(), r); err != nil {
			problem(w, 401, "Sign in to continue")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *API) workspaceRoutes(public, admin, agent *http.ServeMux) {
	public.HandleFunc("GET /auth/status", func(w http.ResponseWriter, r *http.Request) {
		configured, err := a.Sessions.Configured(r.Context())
		if err != nil {
			dbError(w, err)
			return
		}
		email, _ := a.Sessions.Session(r.Context(), r)
		write(w, 200, map[string]any{"configured": configured, "email": email})
	})
	public.HandleFunc("POST /auth/setup", func(w http.ResponseWriter, r *http.Request) {
		if !auth.SameOrigin(r) {
			problem(w, 403, "Same-origin request required")
			return
		}
		configured, err := a.Sessions.Configured(r.Context())
		if err != nil {
			dbError(w, err)
			return
		}
		if configured {
			problem(w, 409, "Owner account already exists. Sign in to continue.")
			return
		}
		if !a.Sessions.AllowAttempt(r.Context(), r) {
			problem(w, 429, "Too many attempts; retry in 15 minutes")
			return
		}
		var body struct {
			Email                string `json:"email"`
			Password             string `json:"password"`
			PasswordConfirmation string `json:"passwordConfirmation"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Password != body.PasswordConfirmation {
			problem(w, 400, "Passwords do not match")
			return
		}
		if err := a.Sessions.Setup(r.Context(), body.Email, body.Password); err != nil {
			if errors.Is(err, auth.ErrSetup) {
				problem(w, 409, err.Error())
			} else {
				problem(w, 400, err.Error())
			}
			return
		}
		token, err := a.Sessions.Login(r.Context(), body.Email, body.Password)
		if err != nil {
			problem(w, 500, "Owner created; sign in to continue")
			return
		}
		a.Sessions.Cookie(w, token)
		write(w, 201, map[string]bool{"ok": true})
	})
	public.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		if !auth.SameOrigin(r) {
			problem(w, 403, "Same-origin request required")
			return
		}
		if !a.Sessions.AllowAttempt(r.Context(), r) {
			problem(w, 429, "Too many attempts; retry in 15 minutes")
			return
		}
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decode(w, r, &body) {
			return
		}
		token, err := a.Sessions.Login(r.Context(), body.Email, body.Password)
		if err != nil {
			problem(w, 401, "Email or password is incorrect")
			return
		}
		a.Sessions.Cookie(w, token)
		write(w, 200, map[string]bool{"ok": true})
	})
	public.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if !auth.SameOrigin(r) {
			problem(w, 403, "Same-origin request required")
			return
		}
		if err := a.Sessions.Logout(r.Context(), r); err != nil {
			dbError(w, err)
			return
		}
		a.Sessions.Cookie(w, "")
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("POST /api/projects/{id}/environments", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &body) {
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if !deployment.ValidName(body.Name) {
			problem(w, 400, "Use a name of 1–60 letters, numbers, spaces, dots, underscores or hyphens")
			return
		}
		v, err := a.Store.CreateEnvironment(r.Context(), r.PathValue("id"), body.Name)
		if err != nil {
			dbError(w, err)
			return
		}
		write(w, 201, v)
	})
	admin.HandleFunc("GET /api/services/{id}/variables", func(w http.ResponseWriter, r *http.Request) {
		items, err := a.Store.Variables(r.Context(), r.PathValue("id"))
		if err != nil {
			dbError(w, err)
			return
		}
		names := make([]string, len(items))
		for index, item := range items {
			names[index] = item.Name
		}
		write(w, 200, map[string]any{"names": names, "variables": items})
	})
	admin.HandleFunc("PUT /api/services/{id}/variables/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Value           string `json:"value"`
			Kind            string `json:"kind"`
			TargetServiceID string `json:"targetServiceId"`
			TargetVariable  string `json:"targetVariable"`
		}
		if !decode(w, r, &body) {
			return
		}
		if strings.ContainsRune(body.Value, 0) {
			problem(w, 400, "Variable values cannot contain null bytes")
			return
		}
		if body.Kind == "" {
			body.Kind = "secret"
		}
		var err error
		if body.Kind == "reference" {
			err = a.Store.SetReference(r.Context(), r.PathValue("id"), r.PathValue("name"), body.TargetServiceID, body.TargetVariable)
		} else {
			err = a.Store.SetVariableTyped(r.Context(), r.PathValue("id"), r.PathValue("name"), body.Value, body.Kind)
		}
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("POST /api/services/{id}/variables/{name}/rename", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &body) {
			return
		}
		if err := a.Store.RenameVariable(r.Context(), r.PathValue("id"), r.PathValue("name"), body.Name); err != nil {
			problem(w, 400, err.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("DELETE /api/services/{id}/variables/{name}", func(w http.ResponseWriter, r *http.Request) {
		config, e := a.Store.Settings(r.Context(), r.PathValue("id"))
		if e != nil {
			dbError(w, e)
			return
		}
		if deployment.IsDataKind(config.Kind) {
			problem(w, 400, "Database template variables are managed by Cloudrail")
			return
		}
		err := a.Store.DeleteVariable(r.Context(), r.PathValue("id"), r.PathValue("name"))
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("POST /api/services/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Kind     string `json:"kind"`
			BackupID string `json:"backupId"`
		}
		if !decode(w, r, &body) {
			return
		}
		v, err := a.Store.EnqueueAction(r.Context(), r.PathValue("id"), body.Kind, body.BackupID)
		if err != nil {
			problem(w, 409, "Service action could not be queued: ensure a release exists and no deployment/action is pending")
			return
		}
		write(w, 202, v)
	})
	admin.HandleFunc("PUT /api/services/{id}/cron", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Schedule string `json:"schedule"`
		}
		if !decode(w, r, &body) {
			return
		}
		service, err := a.Store.SaveCronSchedule(r.Context(), r.PathValue("id"), body.Schedule)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				dbError(w, err)
			} else {
				problem(w, 400, err.Error())
			}
			return
		}
		write(w, 200, service)
	})
	agent.HandleFunc("POST /internal/actions/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		var report deployment.Report
		if !decode(w, r, &report) {
			return
		}
		if len(report.Message) > 1800 {
			problem(w, 400, "Report too long")
			return
		}
		report.Attempt = r.Header.Get("X-Cloudrail-Attempt")
		if err := a.Store.ReportAction(r.Context(), r.PathValue("id"), report); err != nil {
			dbError(w, err)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	agent.HandleFunc("POST /internal/cron-runs/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		var report deployment.Report
		if !decode(w, r, &report) {
			return
		}
		if len(report.Message) > 1800 || len(report.Logs) > 16384 {
			problem(w, 400, "Report exceeds size limit")
			return
		}
		report.Attempt = r.Header.Get("X-Cloudrail-Attempt")
		if err := a.Store.ReportCronRun(r.Context(), r.PathValue("id"), report); err != nil {
			dbError(w, err)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
}
