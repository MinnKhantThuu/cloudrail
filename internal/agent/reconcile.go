package agent

import (
	"cloudrail/internal/deployment"
	"context"
	"errors"
	"fmt"
	"time"
)

func (c *Client) Reconcile(ctx context.Context, r *Runner) error {
	var work []deployment.Work
	if _, e := c.call(ctx, "GET", "/internal/reconcile", nil, &work); e != nil {
		return e
	}
	var failures []error
	for _, w := range work {
		op, cancel := context.WithTimeout(ctx, 15*time.Second)
		w.Deployment.Env = w.Env
		var e error
		if w.Service.ActiveID == "" {
			e = r.Routes.Set(w.Service, nil)
		} else if w.Service.DesiredState == "stopped" {
			e = r.Routes.Set(w.Service, nil)
			if e == nil {
				e = r.Runtime.Stop(op, w.Deployment.ID)
			}
		} else {
			e = r.Runtime.Ensure(op, w.Deployment)
			if e == nil {
				e = r.candidateReady(op, w.Deployment, w.Service)
			}
			if e == nil {
				e = r.Routes.Set(w.Service, &w.Deployment)
			}
		}
		cancel()
		if e != nil {
			failures = append(failures, fmt.Errorf("service %s: reconciliation pending", w.Service.ID))
		}
	}
	return errors.Join(failures...)
}
func (c *Client) WatchCancellation(ctx context.Context, id string, cancel context.CancelFunc) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var status struct {
				Cancelled bool `json:"cancelled"`
			}
			_, e := c.call(ctx, "GET", "/internal/deployments/"+id+"/cancelled", nil, &status)
			if e == nil && status.Cancelled {
				cancel()
				return
			}
		}
	}
}
func (r *Runner) RecoverCancellation(ctx context.Context, w deployment.Work) error {
	return r.fail(ctx, w, errors.New("Deployment cancelled"))
}
