package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"cloudrail/internal/agent"
	"cloudrail/internal/deployment"
	"cloudrail/internal/proxy"
	"cloudrail/internal/runtime/docker"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func main() {
	token := os.Getenv("CLOUDRAIL_AGENT_TOKEN")
	if len(token) < 32 {
		slog.Error("Agent token must have at least 32 characters")
		os.Exit(1)
	}
	stateDir := env("AGENT_STATE_DIR", "/var/lib/cloudrail")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		slog.Error("agent state directory unavailable", "error", err)
		os.Exit(1)
	}
	lock, err := os.OpenFile(filepath.Join(stateDir, "agent.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		slog.Error("agent lock unavailable", "error", err)
		os.Exit(1)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		slog.Error("Another local agent already owns this node")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	stale, _ := filepath.Glob("/tmp/cloudrail-build-*")
	for _, directory := range stale {
		_ = os.RemoveAll(directory)
	}
	client, err := agent.EnrolledClient(ctx, stateDir, env("ENROLLMENT_URL", "http://server:8080"), env("CONTROL_PLANE_URL", "https://server:8443"), token)
	if err != nil {
		slog.Error("node enrollment failed", "error", err)
		os.Exit(1)
	}
	runtime := docker.New(env("DOCKER_SOCKET", "/var/run/docker.sock"), env("APP_NETWORK", "cloudrail-apps"))
	go func() {
		for ctx.Err() == nil {
			op, end := context.WithTimeout(ctx, 8*time.Second)
			metrics := runtime.Metrics(op)
			end()
			if e := client.Heartbeat(ctx, metrics); e != nil && ctx.Err() == nil {
				slog.Warn("node heartbeat failed", "error", e)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	timeout, err := time.ParseDuration(env("READINESS_TIMEOUT", "30s"))
	if err != nil || timeout < time.Second {
		slog.Error("Invalid readiness timeout")
		os.Exit(1)
	}
	redact := agent.Redactor(token)
	runner := &agent.Runner{Runtime: runtime, Routes: proxy.Routes{Directory: env("ROUTES_DIR", "/routes"), HTTPS: os.Getenv("PUBLIC_MODE") == "true"}, Reporter: client, Check: docker.Check, ProxyURL: env("PROXY_URL", "http://proxy:8088"), ReadinessTimeout: timeout, Redact: redact}
	slog.Info("Cloudrail agent ready")
	for ctx.Err() == nil {
		if e := client.Reconcile(ctx, runner); e != nil && ctx.Err() == nil {
			slog.Warn("desired state recovery pending", "error", e)
		}
		w, err := client.Claim(ctx)
		if err == nil && w != nil {
			jobCtx, stop := context.WithTimeout(ctx, 5*time.Minute)
			jobClient := *client
			jobClient.Attempt = w.Attempt
			runner.Reporter = &jobClient
			if w.Action == nil && w.CronRun == nil {
				go jobClient.WatchCancellation(jobCtx, w.Deployment.ID, stop)
			}
			w.Deployment.Env = w.Env
			values := []string{token}
			for _, v := range w.Env {
				values = append(values, v)
			}
			runner.Redact = agent.Redactor(values...)
			if w.Action != nil {
				err = runner.RunAction(jobCtx, *w, &jobClient)
			} else if w.CronRun != nil {
				err = runner.RunCron(jobCtx, *w, &jobClient)
			} else {
				err = runner.Run(jobCtx, *w)
			}
			if jobCtx.Err() != nil && ctx.Err() == nil && w.Action == nil && w.CronRun == nil {
				recovery, end := context.WithTimeout(ctx, 30*time.Second)
				// Cancellation can also mean the bounded attempt timed out. Both must clean the candidate.
				err = runner.RecoverCancellation(recovery, *w)
				end()
			}
			stop()
		} else if err == nil {
			built, buildErr := client.BuildOne(ctx)
			if buildErr != nil {
				slog.Warn("source build pending", "error", buildErr)
			}
			if built {
				continue
			}
			// Catch cleanup interrupted after a committed activation and refresh bounded active logs.
			var stateErr error
			state, stateErr := client.State(ctx)
			if stateErr != nil {
				err = stateErr
			} else {
				for _, d := range state.Deployments {
					if d.Status == "superseded" || d.Status == "failed" {
						op, stop := context.WithTimeout(ctx, 10*time.Second)
						_ = runtime.Remove(op, d.ID)
						_ = runtime.RemovePreDeploy(op, d.ID)
						stop()
					}
					if d.Status == "active" {
						var cron bool
						for _, service := range state.Services {
							if service.ActiveID == d.ID && service.WorkloadMode == "cron" {
								cron = true
								break
							}
						}
						if cron {
							continue
						}
						op, stop := context.WithTimeout(ctx, 10*time.Second)
						logs := redact(runtime.Logs(op, d.ID))
						if logs != d.Logs {
							_ = client.Report(op, d.ID, deployment.Report{Logs: logs})
						}
						stop()
					}
				}
				for _, run := range state.CronRuns {
					if run.Status == "succeeded" || run.Status == "failed" {
						op, stop := context.WithTimeout(ctx, 10*time.Second)
						_ = runtime.RemoveCron(op, run.ID)
						stop()
					}
				}
			}
		}
		if err != nil && ctx.Err() == nil {
			slog.Warn("Deployment work will retry", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
