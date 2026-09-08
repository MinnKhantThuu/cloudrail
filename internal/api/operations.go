package api

import (
	"cloudrail/internal/deployment"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var safeID = regexp.MustCompile(`^[a-f0-9]{24}$`)
var checksumPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (a *API) operationRoutes(admin, agent *http.ServeMux) {
	admin.HandleFunc("PUT /api/services/{id}/runtime", func(w http.ResponseWriter, r *http.Request) {
		var body deployment.RuntimeSettings
		if !decode(w, r, &body) {
			return
		}
		settings, err := a.Store.SaveRuntimeSettings(r.Context(), r.PathValue("id"), body)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				dbError(w, err)
			} else {
				problem(w, 400, err.Error())
			}
			return
		}
		write(w, 200, settings)
	})
	admin.HandleFunc("PUT /api/services/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			MemoryMB  int    `json:"memoryMB"`
			CPUMillis int    `json:"cpuMillis"`
			MountPath string `json:"mountPath"`
		}
		if !decode(w, r, &b) {
			return
		}
		if e := a.Store.SaveSettings(r.Context(), r.PathValue("id"), b.MemoryMB, b.CPUMillis, b.MountPath); e != nil {
			problem(w, 400, e.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("PUT /api/services/{id}/domain", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Host string `json:"host"`
		}
		if !decode(w, r, &b) {
			return
		}
		b.Host = strings.ToLower(strings.TrimSpace(b.Host))
		if !deployment.ValidDomain(b.Host) {
			problem(w, 400, "Invalid hostname")
			return
		}
		if os.Getenv("PUBLIC_MODE") == "true" {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			addresses, e := net.DefaultResolver.LookupIPAddr(ctx, b.Host)
			matches := false
			for _, ip := range addresses {
				if ip.IP.String() == os.Getenv("PUBLIC_IP") {
					matches = true
				}
			}
			if e != nil || !matches {
				problem(w, 400, "Point this domain's DNS directly at this server's public IP first")
				return
			}
		}
		if e := a.Store.SetDomain(r.Context(), r.PathValue("id"), b.Host); e != nil {
			problem(w, 400, e.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("POST /api/projects/{id}/databases", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Name        string `json:"name"`
			Environment string `json:"environment"`
			Template    string `json:"template"`
		}
		if !decode(w, r, &b) {
			return
		}
		if !deployment.ValidName(b.Name) {
			problem(w, 400, "Invalid database service name")
			return
		}
		if b.Template != "" && !deployment.ValidDataTemplate(b.Template) {
			problem(w, 400, "Choose a supported data template")
			return
		}
		v, e := a.Store.CreateDatabase(r.Context(), r.PathValue("id"), b.Name, b.Environment, b.Template)
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 201, v)
	})
	admin.HandleFunc("POST /api/projects/{id}/volumes", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Name        string `json:"name"`
			Environment string `json:"environment"`
		}
		if !decode(w, r, &b) {
			return
		}
		v, e := a.Store.CreateVolume(r.Context(), r.PathValue("id"), b.Environment, strings.TrimSpace(b.Name))
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 201, v)
	})
	admin.HandleFunc("PUT /api/volumes/{id}/attachment", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ServiceID string `json:"serviceId"`
			MountPath string `json:"mountPath"`
		}
		if !decode(w, r, &b) {
			return
		}
		if e := a.Store.AttachVolume(r.Context(), r.PathValue("id"), b.ServiceID, strings.TrimSpace(b.MountPath)); e != nil {
			problem(w, 400, e.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("DELETE /api/volumes/{id}/attachment", func(w http.ResponseWriter, r *http.Request) {
		if e := a.Store.DetachVolume(r.Context(), r.PathValue("id")); e != nil {
			problem(w, 400, e.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	admin.HandleFunc("POST /api/services/{id}/bindings", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Target string `json:"targetServiceId"`
			Name   string `json:"variableName"`
		}
		if !decode(w, r, &b) {
			return
		}
		if e := a.Store.BindDatabase(r.Context(), r.PathValue("id"), b.Target, b.Name); e != nil {
			problem(w, 400, e.Error())
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	list := func(w http.ResponseWriter, r *http.Request) {
		rows, e := a.Store.DB.Query(r.Context(), `SELECT id,service_id,size_bytes,checksum,created_at FROM backups ORDER BY created_at DESC LIMIT 100`)
		if e != nil {
			dbError(w, e)
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, service, checksum string
			var size int64
			var created time.Time
			if e = rows.Scan(&id, &service, &size, &checksum, &created); e != nil {
				dbError(w, e)
				return
			}
			items = append(items, map[string]any{"id": id, "serviceId": service, "size": size, "checksum": checksum, "createdAt": created})
		}
		if e = rows.Err(); e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, items)
	}
	admin.HandleFunc("GET /api/backups", list)
	admin.HandleFunc("GET /api/backups/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !safeID.MatchString(id) {
			problem(w, 404, "Backup not found")
			return
		}
		var kind string
		if e := a.Store.DB.QueryRow(r.Context(), `SELECT s.settings->>'kind' FROM backups b JOIN services s ON s.id=b.service_id WHERE b.id=$1`, id).Scan(&kind); e != nil {
			problem(w, 404, "Backup not found")
			return
		}
		dir := os.Getenv("BACKUP_DIR")
		if dir == "" {
			dir = "/backups"
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		extension := ".dump"
		if kind == "http" {
			extension = ".tar"
		}
		w.Header().Set("Content-Disposition", "attachment; filename=cloudrail-"+id+extension)
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, filepath.Join(dir, id+".dump"))
	})
	agent.HandleFunc("POST /internal/backups", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID        string `json:"id"`
			ServiceID string `json:"serviceId"`
			Size      int64  `json:"size"`
			Checksum  string `json:"checksum"`
		}
		if !decode(w, r, &b) {
			return
		}
		if !safeID.MatchString(b.ID) || !checksumPattern.MatchString(b.Checksum) || b.Size <= 0 {
			problem(w, 400, "Invalid backup metadata")
			return
		}
		var valid bool
		e := a.Store.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM service_actions WHERE id=$1 AND service_id=$2 AND kind='backup' AND status='running' AND attempt_token=$3 AND deadline>now())`, b.ID, b.ServiceID, r.Header.Get("X-Cloudrail-Attempt")).Scan(&valid)
		if e != nil || !valid {
			problem(w, 409, "Backup action is no longer current")
			return
		}
		tag, e := a.Store.DB.Exec(r.Context(), `INSERT INTO backups(id,service_id,size_bytes,checksum) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET id=backups.id WHERE backups.checksum=EXCLUDED.checksum AND backups.size_bytes=EXCLUDED.size_bytes AND backups.service_id=EXCLUDED.service_id`, b.ID, b.ServiceID, b.Size, b.Checksum)
		if e != nil {
			dbError(w, e)
			return
		}
		if tag.RowsAffected() != 1 {
			problem(w, 409, "Backup metadata does not match the existing archive")
			return
		}
		write(w, 201, map[string]bool{"ok": true})
	})
	agent.HandleFunc("GET /internal/backups/{id}", func(w http.ResponseWriter, r *http.Request) {
		var checksum, kind, mount string
		e := a.Store.DB.QueryRow(r.Context(), `SELECT b.checksum,s.settings->>'kind',s.settings->>'mountPath' FROM backups b JOIN services s ON s.id=b.service_id WHERE b.id=$1`, r.PathValue("id")).Scan(&checksum, &kind, &mount)
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, map[string]string{"checksum": checksum, "kind": kind, "mountPath": mount})
	})
	admin.HandleFunc("GET /api/services/{id}/metrics", func(w http.ResponseWriter, r *http.Request) {
		var metrics []byte
		e := a.Store.DB.QueryRow(r.Context(), `SELECT COALESCE((SELECT metrics->'containers'->services.active_id FROM nodes WHERE NOT revoked AND last_seen>now()-interval '20 seconds'),'{}'::jsonb) FROM services WHERE id=$1`, r.PathValue("id")).Scan(&metrics)
		if e != nil {
			dbError(w, e)
			return
		}
		write(w, 200, json.RawMessage(metrics))
	})
}
