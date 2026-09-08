package builds

import (
	"cloudrail/internal/deployment"
	"errors"
	"path"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Repository   string `json:"repository"`
	Installation int64  `json:"installation"`
	Branch       string `json:"branch"`
	Root         string `json:"root"`
	Builder      string `json:"builder"`
	Dockerfile   string `json:"dockerfile"`
	BuildCommand string `json:"buildCommand"`
	StartCommand string `json:"startCommand"`
	Port         int    `json:"port"`
	HealthPath   string `json:"healthPath"`
	AutoDeploy   bool   `json:"autoDeploy"`
}
type Build struct {
	ID           string    `json:"id"`
	ServiceID    string    `json:"serviceId"`
	Commit       string    `json:"commit"`
	Config       Config    `json:"config"`
	Status       string    `json:"status"`
	Image        string    `json:"image"`
	DeploymentID string    `json:"deploymentId"`
	Logs         string    `json:"logs"`
	Error        string    `json:"error"`
	CreatedAt    time.Time `json:"createdAt"`
	Attempt      string    `json:"attempt,omitempty"`
}

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var shaPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

func ValidRepository(s string) bool {
	return repoPattern.MatchString(s) && len(s) <= 200 && !strings.HasPrefix(s, "../") && !strings.HasSuffix(s, "/..") && !strings.HasPrefix(s, "./")
}
func ValidSHA(s string) bool { return shaPattern.MatchString(s) && strings.Trim(s, "0") != "" }
func safePath(s string) bool {
	return len(s) < 256 && !strings.HasPrefix(s, "/") && !strings.ContainsAny(s, "\x00\\\r\n") && s != ".." && !strings.HasPrefix(path.Clean(s), "../")
}
func (c Config) Validate() error {
	if !ValidRepository(c.Repository) || c.Installation < 0 {
		return errors.New("use a GitHub owner/repository and a valid installation")
	}
	if c.Branch == "" || len(c.Branch) > 200 || strings.ContainsAny(c.Branch, "\x00\r\n") {
		return errors.New("invalid branch")
	}
	if !safePath(c.Root) || !safePath(c.Dockerfile) {
		return errors.New("root and Dockerfile must stay within the repository")
	}
	if c.Builder != "dockerfile" && c.Builder != "railpack" {
		return errors.New("choose Dockerfile or Railpack")
	}
	if len(c.BuildCommand) > 1024 || len(c.StartCommand) > 1024 || strings.ContainsAny(c.BuildCommand+c.StartCommand, "\x00") {
		return errors.New("command override too long")
	}
	return (deployment.Spec{Image: "example@sha256:" + strings.Repeat("a", 64), Port: c.Port, HealthPath: c.HealthPath}).Validate()
}
