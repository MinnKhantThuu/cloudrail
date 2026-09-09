package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloudrail/internal/deployment"
	"syscall"
)

type Client struct {
	lastTotal, lastIdle uint64
	samples             map[string]cpuSample
	http                *http.Client
	Network             string
}

func New(socket, network string) *Client {
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &Client{http: &http.Client{Transport: tr}, Network: network}
}
func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker/v1.47"+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.http.Do(req)
}
func responseError(resp *http.Response) error {
	defer resp.Body.Close()
	var b struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&b)
	return fmt.Errorf("Docker returned %d: %s", resp.StatusCode, b.Message)
}
func (c *Client) Pull(ctx context.Context, d deployment.Deployment) error {
	resp, err := c.request(ctx, "GET", "/images/"+url.PathEscape(d.Image)+"/json", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil
	}
	if err = freeDisk(1 << 30); err != nil {
		return err
	}
	resp, err = c.request(ctx, "POST", "/images/create?fromImage="+url.QueryEscape(d.Image), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return responseError(resp)
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var line struct {
			Error string `json:"error"`
		}
		if err = json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return err
		}
		if line.Error != "" {
			return errors.New(line.Error)
		}
	}
	return scanner.Err()
}

type inspection struct {
	Config struct{ Labels map[string]string }
	State  struct {
		Running  bool
		ExitCode int
		Status   string
	}
}

func restartPolicy(settings deployment.Settings) map[string]any {
	policy := settings.RestartPolicy
	if policy == "" {
		policy = "on-failure"
	}
	switch policy {
	case "always":
		return map[string]any{"Name": "unless-stopped", "MaximumRetryCount": 0}
	case "never":
		return map[string]any{"Name": "no", "MaximumRetryCount": 0}
	default:
		maximum := settings.RestartMaxRetries
		if maximum == 0 {
			maximum = 10
		}
		return map[string]any{"Name": "on-failure", "MaximumRetryCount": maximum}
	}
}

func commandOverride(command string) []string {
	if command == "" {
		return nil
	}
	return []string{command}
}

func (c *Client) Ensure(ctx context.Context, d deployment.Deployment) error {
	name := deployment.Container(d.ID)
	resp, err := c.request(ctx, "GET", "/containers/"+name+"/json", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == 200 {
		var v inspection
		err = json.NewDecoder(resp.Body).Decode(&v)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if v.Config.Labels["cloudrail.deployment"] != d.ID {
			return errors.New("container name is owned by another workload")
		}
		if v.State.Running {
			return nil
		}
	} else if resp.StatusCode == 404 {
		resp.Body.Close()
		keys := make([]string, 0, len(d.Env))
		for k := range d.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		env := []string{}
		for _, k := range keys {
			env = append(env, k+"="+d.Env[k])
		}
		if err = freeDisk(512 << 20); err != nil {
			return err
		}
		network := c.Network
		endpoints := map[string]any{network: map[string]any{}}
		if d.Settings.Network != "" {
			if err = c.network(ctx, d.Settings.Network); err != nil {
				return err
			}
			endpoints[d.Settings.Network] = map[string]any{}
		}
		if deployment.IsDataKind(d.Settings.Kind) {
			network = d.Settings.Network
			endpoints = map[string]any{network: map[string]any{"Aliases": []string{"db-" + d.ServiceID}}}
		}
		memory, cpu := d.Settings.MemoryMB, d.Settings.CPUMillis
		if memory == 0 {
			memory = 256
		}
		if cpu == 0 {
			cpu = 1000
		}
		host := map[string]any{"NetworkMode": network, "Memory": int64(memory) * 1024 * 1024, "NanoCpus": int64(cpu) * 1_000_000, "PidsLimit": 256, "CapDrop": []string{"NET_RAW"}, "SecurityOpt": []string{"no-new-privileges:true"}, "RestartPolicy": restartPolicy(d.Settings), "LogConfig": map[string]any{"Type": "json-file", "Config": map[string]string{"max-size": "5m", "max-file": "2"}}}
		if d.Settings.VolumeName != "" {
			if err = c.volume(ctx, d.Settings.VolumeName, d.ServiceID); err != nil {
				return err
			}
			host["Mounts"] = []map[string]any{{"Type": "volume", "Source": d.Settings.VolumeName, "Target": d.Settings.MountPath}}
		}
		body := map[string]any{"Image": d.Image, "Env": env, "Labels": map[string]string{"cloudrail.managed": "true", "cloudrail.deployment": d.ID, "cloudrail.service": d.ServiceID, "cloudrail.port": strconv.Itoa(d.Port), "cloudrail.kind": d.Settings.Kind, "cloudrail.health-path": d.HealthPath}, "HostConfig": host, "NetworkingConfig": map[string]any{"EndpointsConfig": endpoints}}
		if command := commandOverride(d.Settings.StartCommand); command != nil {
			body["Entrypoint"] = []string{"/bin/sh", "-lc"}
			body["Cmd"] = command
		}
		if d.Settings.Kind == "postgres" {
			body["Healthcheck"] = map[string]any{"Test": []string{"CMD", "pg_isready", "-h", "127.0.0.1", "-U", "app", "-d", "app"}, "Interval": int64(2 * time.Second), "Timeout": int64(time.Second), "Retries": 30}
		} else if d.Settings.Kind == "redis" {
			body["Healthcheck"] = map[string]any{"Test": []string{"CMD-SHELL", `REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli ping | grep -q PONG`}, "Interval": int64(2 * time.Second), "Timeout": int64(time.Second), "Retries": 30}
		} else if d.Settings.Kind == "mysql" {
			body["Healthcheck"] = map[string]any{"Test": []string{"CMD-SHELL", `MYSQL_PWD="$MYSQL_PASSWORD" mysqladmin ping -h 127.0.0.1 -u"$MYSQL_USER" --silent`}, "Interval": int64(2 * time.Second), "Timeout": int64(time.Second), "Retries": 60}
		} else if d.Settings.Kind == "mongo" {
			body["Healthcheck"] = map[string]any{"Test": []string{"CMD-SHELL", `mongosh --quiet --host 127.0.0.1 --port 27017 -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval 'quit(db.adminCommand({ping:1}).ok ? 0 : 2)'`}, "Interval": int64(2 * time.Second), "Timeout": int64(2 * time.Second), "Retries": 60}
		}

		resp, err = c.request(ctx, "POST", "/containers/create?name="+name, body)
		if err != nil {
			return err
		}
		if resp.StatusCode != 201 {
			return responseError(resp)
		}
		resp.Body.Close()
	} else {
		return responseError(resp)
	}
	resp, err = c.request(ctx, "POST", "/containers/"+name+"/start", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 && resp.StatusCode != 304 {
		return responseError(resp)
	}
	resp.Body.Close()
	return nil
}
func (c *Client) Running(ctx context.Context, d deployment.Deployment) error {
	stable := time.NewTimer(2 * time.Second)
	defer stable.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		resp, err := c.request(ctx, "GET", "/containers/"+deployment.Container(d.ID)+"/json", nil)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return responseError(resp)
		}
		var state inspection
		err = json.NewDecoder(resp.Body).Decode(&state)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if !state.State.Running {
			return errors.New("worker process exited during readiness")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stable.C:
			return nil
		case <-ticker.C:
		}
	}
}

func (c *Client) RunOnce(ctx context.Context, d deployment.Deployment, runID string) (int, string, error) {
	return c.runOneShot(ctx, d, deployment.CronContainer(runID), "cloudrail.cron-run", runID, d.Settings.StartCommand, true)
}

func (c *Client) PreDeploy(ctx context.Context, d deployment.Deployment) (string, error) {
	name := "cloudrail-predeploy-" + d.ID
	exit, logs, err := c.runOneShot(ctx, d, name, "cloudrail.predeploy", d.ID, d.Settings.PreDeployCommand, false)
	if err != nil {
		return logs, err
	}
	if exit != 0 {
		return logs, fmt.Errorf("pre-deploy process exited with code %d", exit)
	}
	return logs, nil
}

func (c *Client) runOneShot(ctx context.Context, d deployment.Deployment, name, labelKey, labelValue, command string, mountVolume bool) (int, string, error) {
	resp, err := c.request(ctx, "GET", "/containers/"+name+"/json", nil)
	if err != nil {
		return 0, "", err
	}
	created := false
	if resp.StatusCode == http.StatusOK {
		var state inspection
		err = json.NewDecoder(resp.Body).Decode(&state)
		resp.Body.Close()
		if err != nil {
			return 0, "", err
		}
		if state.Config.Labels[labelKey] != labelValue {
			return 0, "", errors.New("one-shot container name is owned by another workload")
		}
		if !state.State.Running && state.State.Status != "created" {
			return state.State.ExitCode, c.containerLogs(ctx, name), nil
		}
		created = state.State.Status == "created"
	} else if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		if err = freeDisk(512 << 20); err != nil {
			return 0, "", err
		}
		network := c.Network
		endpoints := map[string]any{network: map[string]any{}}
		if d.Settings.Network != "" {
			if err = c.network(ctx, d.Settings.Network); err != nil {
				return 0, "", err
			}
			network = d.Settings.Network
			endpoints = map[string]any{network: map[string]any{}}
		}
		keys := make([]string, 0, len(d.Env))
		for key := range d.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		env := make([]string, 0, len(keys))
		for _, key := range keys {
			env = append(env, key+"="+d.Env[key])
		}
		memory, cpu := d.Settings.MemoryMB, d.Settings.CPUMillis
		if memory == 0 {
			memory = 256
		}
		if cpu == 0 {
			cpu = 1000
		}
		host := map[string]any{"NetworkMode": network, "Memory": int64(memory) * 1024 * 1024, "NanoCpus": int64(cpu) * 1_000_000, "PidsLimit": 256, "CapDrop": []string{"NET_RAW"}, "SecurityOpt": []string{"no-new-privileges:true"}, "RestartPolicy": map[string]string{"Name": "no"}, "LogConfig": map[string]any{"Type": "json-file", "Config": map[string]string{"max-size": "5m", "max-file": "2"}}}
		if mountVolume && d.Settings.VolumeName != "" {
			if err = c.volume(ctx, d.Settings.VolumeName, d.ServiceID); err != nil {
				return 0, "", err
			}
			host["Mounts"] = []map[string]any{{"Type": "volume", "Source": d.Settings.VolumeName, "Target": d.Settings.MountPath}}
		}
		body := map[string]any{"Image": d.Image, "Env": env, "Labels": map[string]string{"cloudrail.managed": "true", labelKey: labelValue, "cloudrail.deployment": d.ID, "cloudrail.service": d.ServiceID}, "HostConfig": host, "NetworkingConfig": map[string]any{"EndpointsConfig": endpoints}}
		if override := commandOverride(command); override != nil {
			body["Entrypoint"] = []string{"/bin/sh", "-lc"}
			body["Cmd"] = override
		}
		resp, err = c.request(ctx, "POST", "/containers/create?name="+name, body)
		if err != nil {
			return 0, "", err
		}
		if resp.StatusCode != http.StatusCreated {
			return 0, "", responseError(resp)
		}
		resp.Body.Close()
		created = true
	} else {
		return 0, "", responseError(resp)
	}
	if created {
		resp, err = c.request(ctx, "POST", "/containers/"+name+"/start", nil)
		if err != nil {
			return 0, "", err
		}
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
			return 0, "", responseError(resp)
		}
		resp.Body.Close()
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		resp, err = c.request(ctx, "GET", "/containers/"+name+"/json", nil)
		if err != nil {
			if ctx.Err() != nil {
				logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				logs := c.containerLogs(logCtx, name)
				cancel()
				return 0, logs, ctx.Err()
			}
			return 0, "", err
		}
		if resp.StatusCode != http.StatusOK {
			return 0, "", responseError(resp)
		}
		var state inspection
		err = json.NewDecoder(resp.Body).Decode(&state)
		resp.Body.Close()
		if err != nil {
			return 0, "", err
		}
		if !state.State.Running {
			return state.State.ExitCode, c.containerLogs(ctx, name), nil
		}
		select {
		case <-ctx.Done():
			logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			logs := c.containerLogs(logCtx, name)
			cancel()
			return 0, logs, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) RemoveCron(ctx context.Context, runID string) error {
	resp, err := c.request(ctx, "DELETE", "/containers/"+deployment.CronContainer(runID)+"?force=true", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return responseError(resp)
	}
	resp.Body.Close()
	return nil
}

func (c *Client) RemovePreDeploy(ctx context.Context, deploymentID string) error {
	resp, err := c.request(ctx, "DELETE", "/containers/cloudrail-predeploy-"+deploymentID+"?force=true", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return responseError(resp)
	}
	resp.Body.Close()
	return nil
}

func (c *Client) containerLogs(ctx context.Context, name string) string {
	resp, err := c.request(ctx, "GET", "/containers/"+name+"/logs?stdout=true&stderr=true&tail=200&timestamps=true", nil)
	if err != nil {
		return ""
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return ""
	}
	defer resp.Body.Close()
	var result strings.Builder
	var header [8]byte
	r := io.LimitReader(resp.Body, 256<<10)
	for {
		if _, err = io.ReadFull(r, header[:]); err != nil {
			break
		}
		n := binary.BigEndian.Uint32(header[4:])
		if n > 256<<10 {
			break
		}
		b := make([]byte, n)
		if _, err = io.ReadFull(r, b); err != nil {
			break
		}
		result.Write(b)
	}
	logs := result.String()
	if len(logs) > 16000 {
		logs = logs[len(logs)-16000:]
	}
	return strings.ToValidUTF8(logs, "")
}
func (c *Client) Stop(ctx context.Context, id string) error {
	resp, err := c.request(ctx, "POST", "/containers/"+deployment.Container(id)+"/stop?t=5", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 && resp.StatusCode != 304 && resp.StatusCode != 404 {
		return responseError(resp)
	}
	resp.Body.Close()
	return nil
}
func (c *Client) Remove(ctx context.Context, id string) error {
	resp, err := c.request(ctx, "DELETE", "/containers/"+deployment.Container(id)+"?force=true", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 && resp.StatusCode != 404 {
		return responseError(resp)
	}
	resp.Body.Close()
	return nil
}
func (c *Client) Logs(ctx context.Context, id string) string {
	resp, err := c.request(ctx, "GET", "/containers/"+deployment.Container(id)+"/logs?stdout=true&stderr=true&tail=80&timestamps=true", nil)
	if err != nil {
		return ""
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return ""
	}
	defer resp.Body.Close()
	// Docker's non-TTY streams contain an eight-byte header for each stdout/stderr frame.
	var result strings.Builder
	var header [8]byte
	r := io.LimitReader(resp.Body, 128<<10)
	for {
		if _, err = io.ReadFull(r, header[:]); err != nil {
			break
		}
		n := binary.BigEndian.Uint32(header[4:])
		if n > 128<<10 {
			break
		}
		b := make([]byte, n)
		if _, err = io.ReadFull(r, b); err != nil {
			break
		}
		result.Write(b)
	}
	s := result.String()
	if len(s) > 12000 {
		s = s[len(s)-12000:]
	}
	return strings.ToValidUTF8(s, "")
}
func Check(ctx context.Context, target, host, release string) error {
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	last := "not reachable"
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
		if err != nil {
			return err
		}
		req.Host = host
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 && (release == "" || resp.Header.Get("X-Cloudrail-Deployment") == release) {
				return nil
			}
			last = "HTTP " + strconv.Itoa(resp.StatusCode)
			if release != "" && resp.Header.Get("X-Cloudrail-Deployment") != release {
				last += " (route not yet on requested deployment)"
			}
		} else {
			last = "connection unavailable"
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("readiness timed out: %s", last)
		case <-ticker.C:
		}
	}
}

func freeDisk(minimum uint64) error {
	var v syscall.Statfs_t
	if e := syscall.Statfs("/tmp", &v); e != nil {
		return e
	}
	if v.Bavail*uint64(v.Bsize) < minimum {
		return errors.New("node disk space is below the deployment reserve")
	}
	return nil
}
