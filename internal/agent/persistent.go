package agent

import (
	"cloudrail/internal/deployment"
	"context"
	"errors"
	"fmt"
)

func (r *Runner) candidateReady(ctx context.Context, d deployment.Deployment, s deployment.Service) error {
	if deployment.IsDataKind(d.Settings.Kind) {
		runtime, ok := r.Runtime.(interface {
			Ready(context.Context, deployment.Deployment) error
		})
		if !ok {
			return errors.New("runtime does not support database readiness")
		}
		op, cancel := context.WithTimeout(ctx, r.ReadinessTimeout*4)
		defer cancel()
		return runtime.Ready(op, d)
	}
	if s.WorkloadMode == "worker" {
		runtime, ok := r.Runtime.(interface {
			Running(context.Context, deployment.Deployment) error
		})
		if !ok {
			return errors.New("runtime does not support worker readiness")
		}
		op, cancel := context.WithTimeout(ctx, r.ReadinessTimeout)
		defer cancel()
		return runtime.Running(op, d)
	}
	if s.WorkloadMode == "cron" {
		if s.CronSchedule == "" || s.CronNextRun == nil {
			return errors.New("cron schedule must be configured before activation")
		}
		return nil
	}
	return r.check(ctx, fmt.Sprintf("http://%s:%d%s", deployment.Container(d.ID), d.Port, d.HealthPath), s, "")
}
