package agent

import (
	"cloudrail/internal/builds"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

func (c *Client) BuildOne(ctx context.Context) (bool, error) {
	var b builds.Build
	code, e := c.call(ctx, "POST", "/internal/builds/claim", nil, &b)
	if e != nil || code == 204 {
		return false, e
	}
	worker := *c
	worker.Attempt = b.Attempt
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	logs := &builds.Tail{}
	report := func(ctx context.Context, status, image, message string) error {
		_, e := worker.call(ctx, "POST", "/internal/builds/"+b.ID+"/report", map[string]string{"status": status, "image": image, "logs": logs.String(), "message": message}, nil)
		return e
	}
	var monitor sync.WaitGroup
	monitor.Add(1)
	go func() {
		defer monitor.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				var v struct {
					Cancelled bool `json:"cancelled"`
				}
				_, e := worker.call(runCtx, "GET", "/internal/builds/"+b.ID+"/cancelled", nil, &v)
				if e == nil && v.Cancelled {
					cancel()
					return
				}
				_ = report(runCtx, "building", "", "")
			}
		}
	}()
	var image string
	req, e := http.NewRequestWithContext(runCtx, "GET", c.URL+"/internal/builds/"+b.ID+"/archive", nil)
	if e == nil {
		var response *http.Response
		archiveClient := *c.HTTP
		archiveClient.Timeout = 120 * time.Second
		response, e = archiveClient.Do(req)
		if e == nil {
			if response.StatusCode != 200 {
				e = errors.New("source archive unavailable")
			} else {
				image, e = builds.Run(runCtx, b, io.LimitReader(response.Body, 100<<20), logs)
			}
			response.Body.Close()
		}
	}
	wasCancelled := runCtx.Err() != nil
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	cancel()
	monitor.Wait()
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	status, message := "succeeded", ""
	if e != nil {
		status = "failed"
		message = e.Error()
	}
	if wasCancelled {
		status = "cancelled"
		message = "Build cancelled"
		if timedOut {
			status = "failed"
			message = "Build exceeded the 15 minute execution limit"
		}
	}
	if len(message) > 1800 {
		message = message[:1800]
	}
	message = strings.ToValidUTF8(message, "")
	final, end := context.WithTimeout(ctx, 15*time.Second)
	defer end()
	return true, report(final, status, image, message)
}
