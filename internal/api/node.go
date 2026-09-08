package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

func (a *API) enroll(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(a.AgentToken)) != 1 {
		problem(w, 401, "Enrollment token required")
		return
	}
	var b struct {
		CSR string `json:"csr"`
	}
	if !decode(w, r, &b) {
		return
	}
	cert, serial, hash, e := a.Authority.SignCSR([]byte(b.CSR))
	if e != nil {
		problem(w, 400, "Invalid enrollment request")
		return
	}
	// Repeating the same CSR recovers a lost response; a different key cannot reuse the bootstrap token.
	var saved string
	e = a.Store.DB.QueryRow(r.Context(), `INSERT INTO nodes(id,serial,csr_hash,certificate) VALUES(true,$1,$2,$3) ON CONFLICT(id) DO UPDATE SET csr_hash=nodes.csr_hash WHERE nodes.csr_hash=$2 AND NOT nodes.revoked RETURNING certificate`, serial, hash, cert).Scan(&saved)
	if e != nil {
		problem(w, 409, "Node already enrolled; owner recovery is required")
		return
	}
	write(w, 201, map[string]string{"certificate": saved, "ca": string(a.Authority.PEM)})
}
func (a *API) nodeOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
			problem(w, 401, "Node certificate required")
			return
		}
		var valid bool
		e := a.Store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM nodes WHERE serial=$1 AND NOT revoked)`, r.TLS.PeerCertificates[0].SerialNumber.String()).Scan(&valid)
		if e != nil || !valid {
			problem(w, 401, "Node certificate revoked or unknown")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *API) nodeRoutes(public, admin, agent *http.ServeMux) {
	public.HandleFunc("POST /enroll", a.enroll)
	admin.HandleFunc("GET /api/node", func(w http.ResponseWriter, r *http.Request) {
		var enrolled, revoked, online bool
		var seen string
		var metrics []byte
		e := a.Store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM nodes),COALESCE((SELECT revoked FROM nodes),false),COALESCE((SELECT NOT revoked AND last_seen>now()-interval '20 seconds' FROM nodes),false),COALESCE((SELECT last_seen::text FROM nodes),''),COALESCE((SELECT metrics FROM nodes),'{}'::jsonb)`).Scan(&enrolled, &revoked, &online, &seen, &metrics)
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]any{"enrolled": enrolled, "revoked": revoked, "online": online, "lastSeen": seen, "metrics": json.RawMessage(metrics)})
	})
	admin.HandleFunc("POST /api/node/revoke", func(w http.ResponseWriter, r *http.Request) {
		_, e := a.Store.DB.Exec(r.Context(), `UPDATE nodes SET revoked=true`)
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	agent.HandleFunc("POST /internal/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var metrics json.RawMessage
		if !decode(w, r, &metrics) {
			return
		}
		_, e := a.Store.DB.Exec(r.Context(), `UPDATE nodes SET last_seen=now(),metrics=$1 WHERE NOT revoked`, []byte(metrics))
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
}
