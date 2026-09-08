package docker

import (
	"cloudrail/internal/deployment"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"
)

func (c *Client) network(ctx context.Context, name string) error {
	r, e := c.request(ctx, "GET", "/networks/"+name, nil)
	if e != nil {
		return e
	}
	if r.StatusCode == 200 {
		defer r.Body.Close()
		var v struct{ Labels map[string]string }
		if e = json.NewDecoder(r.Body).Decode(&v); e != nil {
			return e
		}
		if v.Labels["cloudrail.environment"] != "true" {
			return errors.New("private network name belongs to another workload")
		}
		return nil
	}
	if r.StatusCode != 404 {
		return responseError(r)
	}
	r.Body.Close()
	r, e = c.request(ctx, "POST", "/networks/create", map[string]any{"Name": name, "Driver": "bridge", "Internal": true, "CheckDuplicate": true, "Labels": map[string]string{"cloudrail.environment": "true"}})
	if e != nil {
		return e
	}
	if r.StatusCode != 201 {
		return responseError(r)
	}
	r.Body.Close()
	return nil
}
func (c *Client) volume(ctx context.Context, name, service string) error {
	r, e := c.request(ctx, "GET", "/volumes/"+name, nil)
	if e != nil {
		return e
	}
	if r.StatusCode == 200 {
		defer r.Body.Close()
		var v struct{ Labels map[string]string }
		if e = json.NewDecoder(r.Body).Decode(&v); e != nil {
			return e
		}
		if v.Labels["cloudrail.service"] != service {
			return errors.New("volume belongs to another workload")
		}
		return nil
	}
	if r.StatusCode != 404 {
		return responseError(r)
	}
	r.Body.Close()
	r, e = c.request(ctx, "POST", "/volumes/create", map[string]any{"Name": name, "Labels": map[string]string{"cloudrail.managed": "true", "cloudrail.service": service}})
	if e != nil {
		return e
	}
	if r.StatusCode >= 300 {
		return responseError(r)
	}
	r.Body.Close()
	return nil
}
func (c *Client) Ready(ctx context.Context, d deployment.Deployment) error {
	for {
		r, e := c.request(ctx, "GET", "/containers/"+deployment.Container(d.ID)+"/json", nil)
		if e != nil {
			return e
		}
		if r.StatusCode != 200 {
			return responseError(r)
		}
		var v struct {
			State struct {
				Running bool
				Health  struct{ Status string }
			}
		}
		e = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&v)
		r.Body.Close()
		if e != nil {
			return e
		}
		if v.State.Running && v.State.Health.Status == "healthy" {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("database readiness timed out")
		case <-time.After(500 * time.Millisecond):
		}
	}
}
func (c *Client) Exec(ctx context.Context, id string, args []string, stdout io.Writer) error {
	r, e := c.request(ctx, "POST", "/containers/"+deployment.Container(id)+"/exec", map[string]any{"AttachStdout": true, "AttachStderr": true, "AttachStdin": false, "Cmd": args})
	if e != nil {
		return e
	}
	if r.StatusCode != 201 {
		return responseError(r)
	}
	var created struct {
		ID string `json:"Id"`
	}
	e = json.NewDecoder(r.Body).Decode(&created)
	r.Body.Close()
	if e != nil {
		return e
	}
	r, e = c.request(ctx, "POST", "/exec/"+url.PathEscape(created.ID)+"/start", map[string]bool{"Detach": false, "Tty": false})
	if e != nil {
		return e
	}
	if r.StatusCode != 200 {
		return responseError(r)
	}
	e = copyDockerOutput(stdout, r.Body)
	r.Body.Close()
	if e != nil {
		return e
	}
	r, e = c.request(ctx, "GET", "/exec/"+url.PathEscape(created.ID)+"/json", nil)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	var status struct {
		ExitCode int
		Running  bool
	}
	if e = json.NewDecoder(r.Body).Decode(&status); e != nil {
		return e
	}
	if status.Running || status.ExitCode != 0 {
		return fmt.Errorf("database command failed (exit %d)", status.ExitCode)
	}
	return nil
}
