package deployment

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
)

type Environment struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
}
type Action struct {
	BackupID  string    `json:"backupId,omitempty"`
	ID        string    `json:"id"`
	ServiceID string    `json:"serviceId"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"createdAt"`
}
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}
type Volume struct {
	ID                string    `json:"id"`
	ProjectID         string    `json:"projectId"`
	Environment       string    `json:"environment"`
	Name              string    `json:"name"`
	AttachedServiceID string    `json:"attachedServiceId,omitempty"`
	MountPath         string    `json:"mountPath,omitempty"`
	ManagedByTemplate bool      `json:"managedByTemplate"`
	CreatedAt         time.Time `json:"createdAt"`
}
type Service struct {
	URL             string     `json:"url"`
	Settings        Settings   `json:"settings"`
	ID              string     `json:"id"`
	ProjectID       string     `json:"projectId"`
	Name            string     `json:"name"`
	Environment     string     `json:"environment"`
	Host            string     `json:"host"`
	ActiveID        string     `json:"activeId"`
	DesiredState    string     `json:"desiredState"`
	ResourceKind    string     `json:"resourceKind"`
	WorkloadMode    string     `json:"workloadMode"`
	Template        string     `json:"template"`
	TemplateVersion string     `json:"templateVersion,omitempty"`
	CronSchedule    string     `json:"cronSchedule"`
	CronNextRun     *time.Time `json:"cronNextRun,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}
type CronRun struct {
	ID           string     `json:"id"`
	ServiceID    string     `json:"serviceId"`
	DeploymentID string     `json:"deploymentId"`
	ScheduledFor time.Time  `json:"scheduledFor"`
	Status       string     `json:"status"`
	ExitCode     *int       `json:"exitCode,omitempty"`
	Logs         string     `json:"logs"`
	Error        string     `json:"error"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}
type ComputeSpec struct {
	Name         string `json:"name"`
	Environment  string `json:"environment"`
	WorkloadMode string `json:"workloadMode"`
	SourceType   string `json:"sourceType"`
}
type Deployment struct {
	Settings   Settings          `json:"settings"`
	Env        map[string]string `json:"-"`
	ID         string            `json:"id"`
	ServiceID  string            `json:"serviceId"`
	Image      string            `json:"image"`
	Port       int               `json:"port"`
	HealthPath string            `json:"healthPath"`
	Status     string            `json:"status"`
	Error      string            `json:"error"`
	Logs       string            `json:"logs"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}
type Event struct {
	ID           int64     `json:"id"`
	DeploymentID string    `json:"deploymentId"`
	Stage        string    `json:"stage"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"createdAt"`
}
type State struct {
	Environments []Environment `json:"environments"`
	Actions      []Action      `json:"actions"`
	Projects     []Project     `json:"projects"`
	Services     []Service     `json:"services"`
	Deployments  []Deployment  `json:"deployments"`
	Events       []Event       `json:"events"`
	CronRuns     []CronRun     `json:"cronRuns"`
}
type Work struct {
	Attempt    string            `json:"attempt"`
	Env        map[string]string `json:"environment"`
	Action     *Action           `json:"action,omitempty"`
	CronRun    *CronRun          `json:"cronRun,omitempty"`
	Deployment Deployment        `json:"deployment"`
	Service    Service           `json:"service"`
	Previous   *Deployment       `json:"previous,omitempty"`
}
type Report struct {
	Attempt  string `json:"-"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Logs     string `json:"logs"`
	ExitCode *int   `json:"exitCode,omitempty"`
}
type Spec struct {
	Image      string `json:"image"`
	Port       int    `json:"port"`
	HealthPath string `json:"healthPath"`
}

var imagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$`)
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _.-]{0,59}$`)

func ValidName(name string) bool { return namePattern.MatchString(name) }
func (s ComputeSpec) Validate() error {
	if !ValidName(s.Name) {
		return errors.New("use a resource name of 1–60 letters, numbers, spaces, dots, underscores or hyphens")
	}
	if s.Environment == "" || len(s.Environment) > 60 || !namePattern.MatchString(s.Environment) {
		return errors.New("choose a valid environment")
	}
	if s.WorkloadMode != "web" && s.WorkloadMode != "worker" && s.WorkloadMode != "cron" {
		return errors.New("choose web, worker or cron workload mode")
	}
	if s.SourceType != "github" && s.SourceType != "image" && s.SourceType != "empty" {
		return errors.New("choose GitHub, Docker image or empty source")
	}
	return nil
}
func (s Spec) Validate() error {
	if len(s.Image) > 512 || !imagePattern.MatchString(s.Image) {
		return errors.New("use a public image pinned by digest: repository@sha256:<64 lowercase hex characters>")
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("container port must be between 1 and 65535")
	}
	if len(s.HealthPath) > 200 || !strings.HasPrefix(s.HealthPath, "/") || strings.HasPrefix(s.HealthPath, "//") || strings.ContainsAny(s.HealthPath, "\r\n\t #?\\") {
		return errors.New("readiness path must be an absolute path such as /health, without query or fragment")
	}
	return nil
}
func ID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func Terminal(status string) bool {
	return status == "active" || status == "failed" || status == "superseded"
}
func Container(id string) string     { return "cloudrail-app-" + id }
func CronContainer(id string) string { return "cloudrail-cron-" + id }
