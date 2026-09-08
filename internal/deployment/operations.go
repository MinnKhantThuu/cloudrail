package deployment

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

const PostgresImage = "postgres@sha256:ef257d85f76e48da1c64832459b59fcaba1a4dac97bf5d7450c77753542eee94"

type Settings struct {
	Kind                    string `json:"kind"`
	MemoryMB                int    `json:"memoryMB"`
	CPUMillis               int    `json:"cpuMillis"`
	MountPath               string `json:"mountPath"`
	VolumeName              string `json:"volumeName"`
	Network                 string `json:"network"`
	StartCommand            string `json:"startCommand,omitempty"`
	PreDeployCommand        string `json:"preDeployCommand,omitempty"`
	PreDeployTimeoutSeconds int    `json:"preDeployTimeoutSeconds,omitempty"`
	RestartPolicy           string `json:"restartPolicy,omitempty"`
	RestartMaxRetries       int    `json:"restartMaxRetries,omitempty"`
}

func PrivateNetwork(project, environment string) string {
	h := md5.Sum([]byte(project + "/" + environment))
	return "cloudrail-env-" + hex.EncodeToString(h[:8])
}
func VolumeID(service string) string {
	h := md5.Sum([]byte("volume:" + service))
	return hex.EncodeToString(h[:])
}
func (c Settings) Validate() error {
	if c.Kind != "http" && !IsDataKind(c.Kind) {
		return errors.New("invalid service kind")
	}
	min := 64
	if c.Kind == "postgres" {
		min = 256
	}
	if c.MemoryMB < min || c.MemoryMB > 4096 || c.CPUMillis < 100 || c.CPUMillis > 4000 {
		return errors.New("use 64–4096 MB (256+ for PostgreSQL) and 0.1–4 CPUs")
	}
	if c.MountPath != "" {
		if !strings.HasPrefix(c.MountPath, "/") || path.Clean(c.MountPath) != c.MountPath || c.MountPath == "/" || strings.ContainsAny(c.MountPath, "\x00\r\n\\:") {
			return errors.New("invalid volume mount path")
		}
		for _, blocked := range []string{"/proc", "/sys", "/dev", "/etc", "/bin", "/sbin", "/usr", "/var/run"} {
			if c.MountPath == blocked || strings.HasPrefix(c.MountPath, blocked+"/") {
				return errors.New("volume cannot replace system directories")
			}
		}
	}
	if len(c.StartCommand) > 1024 || len(c.PreDeployCommand) > 1024 || strings.ContainsRune(c.StartCommand+c.PreDeployCommand, 0) {
		return errors.New("runtime command is too long or contains a null byte")
	}
	if c.PreDeployCommand == "" {
		if c.PreDeployTimeoutSeconds != 0 {
			return errors.New("pre-deploy timeout requires a command")
		}
	} else if c.PreDeployTimeoutSeconds < 1 || c.PreDeployTimeoutSeconds > 3600 {
		return errors.New("pre-deploy timeout must be 1–3600 seconds")
	}
	if c.RestartPolicy != "" && c.RestartPolicy != "on-failure" && c.RestartPolicy != "always" && c.RestartPolicy != "never" {
		return errors.New("choose On failure, Always or Never restart policy")
	}
	if c.RestartMaxRetries < 0 || c.RestartMaxRetries > 100 {
		return errors.New("restart retries must be between 0 and 100")
	}
	if c.RestartPolicy == "on-failure" && c.RestartMaxRetries < 1 {
		return errors.New("On failure policy requires 1–100 retries")
	}
	return nil
}

type RuntimeSettings struct {
	StartCommand            string `json:"startCommand"`
	PreDeployCommand        string `json:"preDeployCommand"`
	PreDeployTimeoutSeconds int    `json:"preDeployTimeoutSeconds"`
	RestartPolicy           string `json:"restartPolicy"`
	RestartMaxRetries       int    `json:"restartMaxRetries"`
}

func (s *Store) SaveRuntimeSettings(ctx context.Context, id string, runtime RuntimeSettings) (Settings, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var settings Settings
	if err = tx.QueryRow(ctx, `SELECT settings FROM services WHERE id=$1 AND settings->>'kind'='http' FOR UPDATE`, id).Scan(&raw); err != nil {
		return settings, err
	}
	if err = json.Unmarshal(raw, &settings); err != nil {
		return settings, err
	}
	settings.StartCommand = strings.TrimSpace(runtime.StartCommand)
	settings.PreDeployCommand = strings.TrimSpace(runtime.PreDeployCommand)
	settings.PreDeployTimeoutSeconds = runtime.PreDeployTimeoutSeconds
	settings.RestartPolicy = runtime.RestartPolicy
	settings.RestartMaxRetries = runtime.RestartMaxRetries
	if settings.RestartPolicy == "" {
		settings.RestartPolicy = "on-failure"
	}
	if settings.RestartPolicy == "on-failure" && settings.RestartMaxRetries == 0 {
		settings.RestartMaxRetries = 10
	}
	if settings.PreDeployCommand == "" {
		settings.PreDeployTimeoutSeconds = 0
	}
	if err = settings.Validate(); err != nil {
		return settings, err
	}
	raw, _ = json.Marshal(settings)
	tag, err := tx.Exec(ctx, `UPDATE services SET settings=$2 WHERE id=$1`, id, raw)
	if err != nil {
		return settings, err
	}
	if tag.RowsAffected() != 1 {
		return settings, pgx.ErrNoRows
	}
	return settings, tx.Commit(ctx)
}
func (s *Store) Settings(ctx context.Context, id string) (Settings, error) {
	var raw []byte
	var c Settings
	e := s.DB.QueryRow(ctx, `SELECT settings FROM services WHERE id=$1`, id).Scan(&raw)
	if e == nil {
		e = json.Unmarshal(raw, &c)
	}
	return c, e
}
func (s *Store) SaveSettings(ctx context.Context, id string, memory, cpu int, mount string) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var project, environment string
	e = tx.QueryRow(ctx, `SELECT settings,project_id,environment FROM services WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &project, &environment)
	if e != nil {
		return e
	}
	var old Settings
	if e = json.Unmarshal(raw, &old); e != nil {
		return e
	}
	if old.MountPath != "" && mount != old.MountPath {
		return errors.New("an attached volume cannot be moved or detached in this release")
	}
	old.MemoryMB = memory
	old.CPUMillis = cpu
	old.MountPath = mount
	old.Network = PrivateNetwork(project, environment)
	if mount != "" {
		old.VolumeName = "cloudrail-volume-" + id
	}
	if e = old.Validate(); e != nil {
		return e
	}
	raw, _ = json.Marshal(old)
	_, e = tx.Exec(ctx, `UPDATE services SET settings=$2 WHERE id=$1`, id, raw)
	if e != nil {
		return e
	}
	if old.VolumeName != "" {
		volumeID := VolumeID(id)
		if _, e = tx.Exec(ctx, `INSERT INTO volumes(id,project_id,environment,name) VALUES($1,$2,$3,$4)
		 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name`, volumeID, project, environment, old.VolumeName); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO volume_attachments(volume_id,service_id,mount_path) VALUES($1,$2,$3)
		 ON CONFLICT(volume_id) DO UPDATE SET service_id=EXCLUDED.service_id,mount_path=EXCLUDED.mount_path`, volumeID, id, old.MountPath); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

var domainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func ValidDomain(host string) bool { return len(host) <= 253 && domainPattern.MatchString(host) }
func (s *Store) SetDomain(ctx context.Context, id, host string) error {
	if !ValidDomain(host) {
		return errors.New("enter a hostname without scheme, port or path")
	}
	if host == os.Getenv("DASHBOARD_DOMAIN") {
		return errors.New("dashboard domain is reserved")
	}
	tag, e := s.DB.Exec(ctx, `UPDATE services SET host=$2 WHERE id=$1 AND settings->>'kind'='http'`, id, host)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return errors.New("HTTP service not found")
	}
	return nil
}
func (s *Store) CreateDatabase(ctx context.Context, project, name, environment string, templateKeys ...string) (Service, error) {
	if environment == "" {
		environment = "production"
	}
	templateKey := "postgres"
	if len(templateKeys) > 0 && templateKeys[0] != "" {
		templateKey = templateKeys[0]
	}
	template, e := dataTemplate(templateKey, "")
	if e != nil {
		return Service{}, e
	}
	v := Service{ID: ID(), ProjectID: project, Name: name, Environment: environment, DesiredState: "running", ResourceKind: "database", WorkloadMode: "web", Template: template.Key, TemplateVersion: template.Version}
	v.Host = v.ID + ".localhost"
	v.Settings = Settings{Kind: template.Kind, MemoryMB: template.MemoryMB, CPUMillis: 1000, MountPath: template.MountPath, VolumeName: "cloudrail-volume-" + v.ID, Network: PrivateNetwork(project, environment), StartCommand: template.StartCommand, RestartPolicy: "always"}
	raw, _ := json.Marshal(v.Settings)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return v, e
	}
	defer tx.Rollback(ctx)
	e = tx.QueryRow(ctx, `INSERT INTO services(id,project_id,name,environment,host,settings,resource_kind,template_key,template_version) VALUES($1,$2,$3,$4,$5,$6,'database',$7,$8) RETURNING created_at`, v.ID, project, name, environment, v.Host, raw, template.Key, template.Version).Scan(&v.CreatedAt)
	if e != nil {
		return v, e
	}
	volumeID := VolumeID(v.ID)
	if _, e = tx.Exec(ctx, `INSERT INTO volumes(id,project_id,environment,name) VALUES($1,$2,$3,$4)`, volumeID, project, environment, v.Settings.VolumeName); e != nil {
		return v, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO volume_attachments(volume_id,service_id,mount_path) VALUES($1,$2,$3)`, volumeID, v.ID, v.Settings.MountPath); e != nil {
		return v, e
	}
	for k, value := range template.Variables(v.ID) {
		encrypted, err := s.Cipher.Seal(value, "variable:"+v.ID+":"+k)
		if err != nil {
			return v, err
		}
		if _, e = tx.Exec(ctx, `INSERT INTO service_variables(service_id,name,ciphertext) VALUES($1,$2,$3)`, v.ID, k, encrypted); e != nil {
			return v, e
		}
	}
	if _, e = s.enqueueTx(ctx, tx, v.ID, Spec{Image: template.Image, Port: template.Port, HealthPath: "/"}, "database-bootstrap"); e != nil {
		return v, e
	}
	return v, tx.Commit(ctx)
}
func (s *Store) BindDatabase(ctx context.Context, database, target, name string) error {
	if !variableName.MatchString(name) {
		return errors.New("invalid variable name")
	}
	var same bool
	e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM services d JOIN services a ON d.project_id=a.project_id AND d.environment=a.environment WHERE d.id=$1 AND a.id=$2 AND d.resource_kind='database' AND a.settings->>'kind'='http')`, database, target).Scan(&same)
	if e != nil {
		return e
	}
	if !same {
		return errors.New("choose an HTTP service in the same project/environment")
	}
	var project, environment, templateKey string
	if e = s.DB.QueryRow(ctx, `SELECT a.project_id,a.environment,d.template_key FROM services a CROSS JOIN services d WHERE a.id=$1 AND d.id=$2`, target, database).Scan(&project, &environment, &templateKey); e != nil {
		return e
	}
	if _, e = s.DB.Exec(ctx, `UPDATE services SET settings=jsonb_set(settings,'{network}',to_jsonb($2::text)) WHERE id=$1`, target, PrivateNetwork(project, environment)); e != nil {
		return e
	}
	vars, e := s.variables(ctx, s.DB, database)
	if e != nil {
		return e
	}
	connection, targetVariable := "", ""
	switch templateKey {
	case "postgres":
		password := vars["POSTGRES_PASSWORD"]
		if password != "" {
			connection = "postgres://app:" + password + "@db-" + database + ":5432/app?sslmode=disable"
		}
		targetVariable = "DATABASE_URL"
	case "redis":
		connection = vars["REDIS_URL"]
		targetVariable = "REDIS_URL"
	}
	if connection == "" {
		return errors.New("database credentials unavailable")
	}
	if e = s.SetVariable(ctx, target, name, connection); e != nil {
		return e
	}
	_, e = s.DB.Exec(ctx, `INSERT INTO service_references(source_service_id,variable_name,target_service_id,target_variable) VALUES($1,$2,$3,$4)
	 ON CONFLICT(source_service_id,variable_name) DO UPDATE SET target_service_id=EXCLUDED.target_service_id,target_variable=EXCLUDED.target_variable,created_at=now()`, target, name, database, targetVariable)
	return e
}
