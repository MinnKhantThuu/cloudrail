package agent

import (
	"cloudrail/internal/deployment"
	"context"
	"errors"
	"fmt"
)

func (r *Runner) candidateReady(ctx context.Context, d deployment.Deployment, s deployment.Service) error {
	if d.Settings.Kind == "postgres" {
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
	return r.check(ctx, fmt.Sprintf("http://%s:%d%s", deployment.Container(d.ID), d.Port, d.HealthPath), s, "")
}
