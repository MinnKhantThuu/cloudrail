package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"cloudrail/internal/deployment"
)

type Client struct {
	URL, Token string
	Attempt    string
	HTTP       *http.Client
}

func NewClient(url, token string) *Client {
	return &Client{URL: strings.TrimRight(url, "/"), Token: token, HTTP: &http.Client{Timeout: 15 * time.Second}}
}
func (c *Client) call(ctx context.Context, method, path string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	if c.Attempt != "" {
		req.Header.Set("X-Cloudrail-Attempt", c.Attempt)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("control plane returned HTTP %d", resp.StatusCode)
	}
	if out != nil && resp.StatusCode != 204 {
		err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
	}
	return resp.StatusCode, err
}
func (c *Client) Claim(ctx context.Context) (*deployment.Work, error) {
	var w deployment.Work
	code, err := c.call(ctx, "POST", "/internal/claim", nil, &w)
	if err != nil {
		return nil, err
	}
	if code == 204 {
		return nil, nil
	}
	return &w, nil
}
func (c *Client) Report(ctx context.Context, id string, r deployment.Report) error {
	_, err := c.call(ctx, "POST", "/internal/deployments/"+id+"/report", r, nil)
	return err
}
func (c *Client) ReportCronRun(ctx context.Context, id string, r deployment.Report) error {
	_, err := c.call(ctx, "POST", "/internal/cron-runs/"+id+"/report", r, nil)
	return err
}
func (c *Client) State(ctx context.Context) (deployment.State, error) {
	var s deployment.State
	_, err := c.call(ctx, "GET", "/internal/state", nil, &s)
	return s, err
}

var sensitive = regexp.MustCompile(`(?i)(authorization[=: ]+|bearer +|password[=: ]+|token[=: ]+|secret[=: ]+)[^\s,;]+`)

func Redactor(known ...string) func(string) string {
	return func(s string) string {
		for _, value := range known {
			if value != "" {
				s = strings.ReplaceAll(s, value, "[REDACTED]")
			}
		}
		return sensitive.ReplaceAllString(s, "${1}[REDACTED]")
	}
}
