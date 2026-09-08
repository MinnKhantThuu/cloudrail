package api

import "net/http"

func (a *API) reliabilityRoutes(admin, agent *http.ServeMux) {
	admin.HandleFunc("POST /api/deployments/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if e := a.Store.Cancel(r.Context(), r.PathValue("id")); e != nil {
			dbError(w, e)
			return
		}
		write(w, 202, map[string]bool{"ok": true})
	})
	agent.HandleFunc("GET /internal/deployments/{id}/cancelled", func(w http.ResponseWriter, r *http.Request) {
		var cancel bool
		e := a.Store.DB.QueryRow(r.Context(), `SELECT cancel_requested FROM deployments WHERE id=$1`, r.PathValue("id")).Scan(&cancel)
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]bool{"cancelled": cancel})
	})
	agent.HandleFunc("GET /internal/reconcile", func(w http.ResponseWriter, r *http.Request) {
		work, e := a.Store.ReconcileWork(r.Context())
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, work)
	})
}
