package agent

import (
	"bytes"
	"cloudrail/internal/builds"
	"cloudrail/internal/deployment"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type backupRuntime interface {
	Exec(context.Context, string, []string, io.Writer) error
	PutRestore(context.Context, string, string) error
	ExportVolume(context.Context, deployment.Deployment, io.Writer) error
	RestoreVolume(context.Context, deployment.Deployment, string, string) error
	RestoreRedisVolume(context.Context, deployment.Deployment, string, string) error
}

func (r *Runner) RunBackup(ctx context.Context, w deployment.Work, c *Client) error {
	runtime, ok := r.Runtime.(backupRuntime)
	if !ok {
		return errors.New("runtime does not support backups")
	}
	dir := os.Getenv("BACKUP_DIR")
	if dir == "" {
		dir = "/backups"
	}
	var backupID = w.Action.ID
	operation := func() error {
		var e error
		if w.Action.Kind == "backup" {
			if w.Deployment.Settings.Kind == "postgres" {
				var size bytes.Buffer
				if e := runtime.Exec(ctx, w.Deployment.ID, []string{"psql", "-U", "app", "-d", "app", "-Atc", "SELECT pg_database_size(current_database())"}, &size); e != nil {
					return e
				}
				estimated, e := strconv.ParseUint(strings.TrimSpace(size.String()), 10, 64)
				if e != nil {
					return e
				}
				if e = builds.CheckDisk(dir, estimated*2+(256<<20)); e != nil {
					return e
				}
			} else if e := builds.CheckDisk(dir, 1<<30); e != nil {
				return e
			}
			file := filepath.Join(dir, backupID+".dump")
			if _, e = os.Stat(file); errors.Is(e, os.ErrNotExist) {
				f, e := os.OpenFile(file+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
				if errors.Is(e, os.ErrExist) {
					_ = os.Remove(file + ".tmp")
					f, e = os.OpenFile(file+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
				}
				if e != nil {
					return e
				}
				defer os.Remove(file + ".tmp")
				if w.Deployment.Settings.Kind == "postgres" {
					e = runtime.Exec(ctx, w.Deployment.ID, []string{"pg_dump", "-U", "app", "-d", "app", "-Fc", "--no-owner", "--no-privileges"}, f)
				} else {
					e = runtime.ExportVolume(ctx, w.Deployment, f)
				}
				if e == nil {
					e = f.Sync()
				}
				f.Close()
				if e != nil {
					return e
				}
				if e = os.Chown(file+".tmp", 0, 10001); e != nil {
					return e
				}
				if e = os.Rename(file+".tmp", file); e != nil {
					return e
				}
			} else if e != nil {
				return e
			}
			f, e := os.Open(file)
			if e != nil {
				return e
			}
			h := sha256.New()
			n, e := io.Copy(h, f)
			f.Close()
			if e != nil {
				return e
			}
			_, e = c.call(ctx, "POST", "/internal/backups", map[string]any{"id": backupID, "serviceId": w.Service.ID, "size": n, "checksum": hex.EncodeToString(h.Sum(nil))}, nil)
			return e
		}
		backupID = w.Action.BackupID
		var meta struct {
			Checksum        string `json:"checksum"`
			Kind            string `json:"kind"`
			MountPath       string `json:"mountPath"`
			TemplateVersion string `json:"templateVersion"`
		}
		if _, e := c.call(ctx, "GET", "/internal/backups/"+backupID, nil, &meta); e != nil {
			return e
		}
		file := filepath.Join(dir, backupID+".dump")
		f, e := os.Open(file)
		if e != nil {
			return e
		}
		h := sha256.New()
		_, e = io.Copy(h, f)
		f.Close()
		if e != nil {
			return e
		}
		if hex.EncodeToString(h.Sum(nil)) != meta.Checksum {
			return errors.New("backup checksum mismatch")
		}
		if meta.Kind != w.Deployment.Settings.Kind {
			return errors.New("backup and target workload types do not match")
		}
		if meta.Kind == "http" {
			return runtime.RestoreVolume(ctx, w.Deployment, file, meta.MountPath)
		}
		if meta.Kind == "redis" {
			if meta.TemplateVersion == "" || meta.TemplateVersion != w.Service.TemplateVersion {
				return errors.New("Redis backup and target template versions do not match")
			}
			if e = r.Runtime.Ensure(ctx, w.Deployment); e != nil {
				return e
			}
			stop := true
			defer func() {
				if stop {
					cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					_ = r.Runtime.Stop(cleanup, w.Deployment.ID)
				}
			}()
			if e = r.candidateReady(ctx, w.Deployment, w.Service); e != nil {
				return e
			}
			var existing bytes.Buffer
			if e = runtime.Exec(ctx, w.Deployment.ID, []string{"sh", "-lc", `REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli DBSIZE`}, &existing); e != nil {
				return e
			}
			if strings.TrimSpace(existing.String()) != "0" {
				return errors.New("restore requires an empty target Redis service; existing keys were preserved")
			}
			if e = r.Runtime.Stop(ctx, w.Deployment.ID); e != nil {
				return e
			}
			stop = false
			return runtime.RestoreRedisVolume(ctx, w.Deployment, file, meta.MountPath)
		}
		var existing bytes.Buffer
		if e = runtime.Exec(ctx, w.Deployment.ID, []string{"psql", "-U", "app", "-d", "app", "-Atc", "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND c.relkind IN ('r','p','m','S','v')"}, &existing); e != nil {
			return e
		}
		if strings.TrimSpace(existing.String()) != "0" {
			return errors.New("restore requires an empty target database; create a new PostgreSQL service")
		}
		if e = runtime.PutRestore(ctx, w.Deployment.ID, file); e != nil {
			return e
		}
		defer runtime.Exec(ctx, w.Deployment.ID, []string{"rm", "-f", "/tmp/cloudrail-restore.dump"}, io.Discard)
		return runtime.Exec(ctx, w.Deployment.ID, []string{"pg_restore", "-U", "app", "-d", "app", "--single-transaction", "--exit-on-error", "--no-owner", "--no-privileges", "/tmp/cloudrail-restore.dump"}, io.Discard)
	}
	if e := os.MkdirAll(dir, 0750); e != nil {
		return e
	}
	_ = os.Chown(dir, 0, 10001)
	_ = os.Chmod(dir, 0750)
	err := operation()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	report := deployment.Report{Status: "done"}
	if err != nil {
		report.Status = "failed"
		report.Message = err.Error()
		if len(report.Message) > 1700 {
			report.Message = report.Message[:1700]
		}
	}
	return c.ReportAction(ctx, w.Action.ID, report)
}
