package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloudrail/internal/deployment"
)

type Runtime interface {
	Pull(context.Context, deployment.Deployment) error
	Ensure(context.Context, deployment.Deployment) error
	Stop(context.Context, string) error
	Remove(context.Context, string) error
	Logs(context.Context, string) string
}
type CronRuntime interface {
	RunOnce(context.Context, deployment.Deployment, string) (int, string, error)
	RemoveCron(context.Context, string) error
}
type CronReporter interface {
	ReportCronRun(context.Context, string, deployment.Report) error
}
type PreDeployRuntime interface {
	PreDeploy(context.Context, deployment.Deployment) (string, error)
	RemovePreDeploy(context.Context, string) error
}
type Router interface {
	Set(deployment.Service, *deployment.Deployment) error
}
type Reporter interface {
	Report(context.Context, string, deployment.Report) error
}
type Runner struct {
	Runtime          Runtime
	Routes           Router
	Reporter         Reporter
	Check            func(context.Context, string, string, string) error
	ProxyURL         string
	ReadinessTimeout time.Duration
	Redact           func(string) string
}

func webWorkload(s deployment.Service) bool {
	return s.PublicHTTP()
}

func (r *Runner) setRoute(s deployment.Service, d *deployment.Deployment) error {
	if !webWorkload(s) {
		return r.Routes.Set(s, nil)
	}
	return r.Routes.Set(s, d)
}

func (r *Runner) report(ctx context.Context, d deployment.Deployment, status, message string) error {
	logs := r.Runtime.Logs(ctx, d.ID)
	if r.Redact != nil {
		message = r.Redact(message)
		logs = r.Redact(logs)
	}
	if len(message) > 1800 {
		message = message[:1800]
	}
	if len(logs) > 16000 {
		logs = logs[len(logs)-16000:]
	}
	return r.Reporter.Report(ctx, d.ID, deployment.Report{Status: status, Message: strings.ToValidUTF8(message, ""), Logs: strings.ToValidUTF8(logs, "")})
}
func (r *Runner) check(ctx context.Context, target string, s deployment.Service, id string) error {
	c, cancel := context.WithTimeout(ctx, r.ReadinessTimeout)
	defer cancel()
	return r.Check(c, target, s.Host, id)
}
func (r *Runner) fail(ctx context.Context, w deployment.Work, cause error) error {
	return r.failWithLogs(ctx, w, cause, r.Runtime.Logs(ctx, w.Deployment.ID))
}
func (r *Runner) failWithLogs(ctx context.Context, w deployment.Work, cause error, logs string) error {
	// A shared volume must have only one writer, including during recovery.
	if w.Deployment.Settings.VolumeName != "" {
		if e := r.Runtime.Remove(ctx, w.Deployment.ID); e != nil {
			return e
		}
		if w.Previous != nil {
			if e := r.Runtime.Ensure(ctx, *w.Previous); e != nil {
				return e
			}
			if e := r.candidateReady(ctx, *w.Previous, w.Service); e != nil {
				return e
			}
		}
	}
	// A resumed attempt may already have changed the route. Always restore before removing it.
	if err := r.setRoute(w.Service, w.Previous); err != nil {
		return fmt.Errorf("route recovery pending: %w", err)
	}
	if w.Previous != nil && webWorkload(w.Service) {
		if err := r.check(ctx, r.ProxyURL+w.Previous.HealthPath, w.Service, w.Previous.ID); err != nil {
			return fmt.Errorf("previous route verification pending: %w", err)
		}
	}
	if err := r.Runtime.Remove(ctx, w.Deployment.ID); err != nil {
		return fmt.Errorf("candidate cleanup pending: %w", err)
	}
	message := cause.Error()
	if r.Redact != nil {
		message = r.Redact(message)
		logs = r.Redact(logs)
	}
	if len(message) > 1800 {
		message = message[:1800]
	}
	if len(logs) > 16000 {
		logs = logs[len(logs)-16000:]
	}
	return r.Reporter.Report(ctx, w.Deployment.ID, deployment.Report{Status: "failed", Message: strings.ToValidUTF8(message, ""), Logs: strings.ToValidUTF8(logs, "")})
}
func (r *Runner) Run(ctx context.Context, w deployment.Work) error {
	d := w.Deployment
	if err := r.report(ctx, d, "pulling", "Fetching pinned image"); err != nil {
		return err
	}
	pullCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	err := r.Runtime.Pull(pullCtx, d)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return r.fail(ctx, w, fmt.Errorf("image pull failed: %w", err))
	}
	if d.Settings.PreDeployCommand != "" {
		runtime, ok := r.Runtime.(PreDeployRuntime)
		if !ok {
			return r.fail(ctx, w, errors.New("runtime does not support pre-deploy commands"))
		}
		if err = r.Reporter.Report(ctx, d.ID, deployment.Report{Status: "predeploy", Message: "Running pre-deploy command in an isolated container"}); err != nil {
			return err
		}
		timeout := time.Duration(d.Settings.PreDeployTimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		preCtx, stop := context.WithTimeout(ctx, timeout)
		logs, preErr := runtime.PreDeploy(preCtx, d)
		stop()
		if preErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = runtime.RemovePreDeploy(cleanup, d.ID)
			cancel()
			return r.failWithLogs(ctx, w, fmt.Errorf("pre-deploy command failed: %w", preErr), logs)
		}
		if err = runtime.RemovePreDeploy(ctx, d.ID); err != nil {
			return err
		}
	}
	if err = r.report(ctx, d, "starting", "Starting isolated candidate container"); err != nil {
		return err
	}
	if w.Service.WorkloadMode == "cron" {
		if err = r.setRoute(w.Service, nil); err != nil {
			return r.fail(ctx, w, fmt.Errorf("route cleanup failed: %w", err))
		}
		if err = r.report(ctx, d, "active", "Cron release is ready for its next scheduled run"); err != nil {
			return err
		}
		if w.Previous != nil {
			_ = r.Runtime.Remove(ctx, w.Previous.ID)
		}
		return nil
	}
	if d.Settings.VolumeName != "" && w.Previous != nil {
		if err = r.Routes.Set(w.Service, nil); err != nil {
			return err
		}
		if err = r.Runtime.Stop(ctx, w.Previous.ID); err != nil {
			return err
		}
	}
	if err = r.Runtime.Ensure(ctx, d); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return r.fail(ctx, w, fmt.Errorf("container start failed: %w", err))
	}
	if err = r.report(ctx, d, "checking", "Checking candidate readiness; current release stays online"); err != nil {
		return err
	}
	if err = r.candidateReady(ctx, d, w.Service); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return r.fail(ctx, w, err)
	}
	activation := "Activating private release"
	if webWorkload(w.Service) {
		activation = "Switching route and verifying through Traefik"
	}
	if err = r.report(ctx, d, "routing", activation); err != nil {
		return err
	}
	if err = r.setRoute(w.Service, &d); err != nil {
		return r.fail(ctx, w, fmt.Errorf("route update failed: %w", err))
	}
	if webWorkload(w.Service) {
		if err = r.check(ctx, r.ProxyURL+d.HealthPath, w.Service, d.ID); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return r.fail(ctx, w, err)
		}
	}
	// Keep both containers if the acknowledgement fails. The same job is safe to resume.
	activeMessage := "Deployment is active on the private network"
	if webWorkload(w.Service) {
		activeMessage = "Deployment is serving through the proxy"
	}
	if err = r.report(ctx, d, "active", activeMessage); err != nil {
		return err
	}
	if w.Previous != nil {
		if err = r.Runtime.Stop(ctx, w.Previous.ID); err != nil {
			slog.Warn("old release cleanup deferred", "deployment", w.Previous.ID, "error", err)
		}
	}
	return nil
}

func (r *Runner) RunCron(ctx context.Context, w deployment.Work, client CronReporter) error {
	if w.CronRun == nil {
		return errors.New("cron work has no run")
	}
	runtime, ok := r.Runtime.(CronRuntime)
	if !ok {
		return errors.New("runtime does not support one-shot jobs")
	}
	w.Deployment.Env = w.Env
	exit, logs, err := runtime.RunOnce(ctx, w.Deployment, w.CronRun.ID)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	report := deployment.Report{Status: "succeeded", Logs: logs, ExitCode: &exit}
	if err != nil || exit != 0 {
		report.Status = "failed"
		if err != nil {
			report.Message = "Cron run failed: " + err.Error()
		} else {
			report.Message = fmt.Sprintf("Cron process exited with code %d", exit)
		}
	}
	if r.Redact != nil {
		report.Logs = r.Redact(report.Logs)
		report.Message = r.Redact(report.Message)
	}
	if err = client.ReportCronRun(ctx, w.CronRun.ID, report); err != nil {
		return err
	}
	cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = runtime.RemoveCron(cleanup, w.CronRun.ID); err != nil {
		slog.Warn("cron container cleanup deferred", "run", w.CronRun.ID, "error", err)
	}
	return nil
}
