package agent

import (
	"cloudrail/internal/deployment"
	"context"
)

func (c *Client) ReportAction(ctx context.Context, id string, r deployment.Report) error {
	_, err := c.call(ctx, "POST", "/internal/actions/"+id+"/report", r, nil)
	return err
}
func (r *Runner) RunAction(ctx context.Context, w deployment.Work, client *Client) error {
	d := w.Deployment
	a := w.Action
	if a.Kind == "backup" || a.Kind == "restore" {
		return r.RunBackup(ctx, w, client)
	}
	operation := func() error {
		if err := r.Routes.Set(w.Service, nil); err != nil {
			return err
		}
		if a.Kind == "stop" || a.Kind == "restart" {
			if err := r.Runtime.Stop(ctx, d.ID); err != nil {
				return err
			}
		}
		if a.Kind == "stop" {
			return nil
		}
		if err := r.Runtime.Ensure(ctx, d); err != nil {
			return err
		}
		if err := r.candidateReady(ctx, d, w.Service); err != nil {
			return err
		}
		if err := r.Routes.Set(w.Service, &d); err != nil {
			return err
		}
		if d.Settings.Kind == "postgres" {
			return nil
		}
		return r.check(ctx, r.ProxyURL+d.HealthPath, w.Service, d.ID)
	}
	err := operation()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	report := deployment.Report{Status: "done"}
	if err != nil {
		report.Status = "failed"
		report.Message = "Service action failed; inspect the current release logs and retry"
		_ = r.Routes.Set(w.Service, nil)
	}
	return client.ReportAction(ctx, a.ID, report)
}
