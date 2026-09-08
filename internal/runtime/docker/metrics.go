package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ContainerMetric struct {
	Running bool     `json:"running"`
	Status  string   `json:"status"`
	Memory  *uint64  `json:"memoryBytes"`
	CPU     *float64 `json:"cpuPercent"`
}
type Metrics struct {
	MemoryTotal     uint64                     `json:"memoryTotal"`
	MemoryAvailable uint64                     `json:"memoryAvailable"`
	DiskFree        uint64                     `json:"diskFree"`
	DiskTotal       uint64                     `json:"diskTotal"`
	CPU             float64                    `json:"cpuPercent"`
	Containers      map[string]ContainerMetric `json:"containers"`
}

type cpuSample struct {
	CPU uint64
	At  time.Time
}

func (c *Client) Metrics(ctx context.Context) Metrics {
	if c.samples == nil {
		c.samples = map[string]cpuSample{}
	}
	m := Metrics{Containers: map[string]ContainerMetric{}}
	mem, _ := os.ReadFile("/proc/meminfo")
	for _, line := range strings.Split(string(mem), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 {
			v, _ := strconv.ParseUint(fields[1], 10, 64)
			if fields[0] == "MemTotal:" {
				m.MemoryTotal = v * 1024
			}
			if fields[0] == "MemAvailable:" {
				m.MemoryAvailable = v * 1024
			}
		}
	}
	var disk syscall.Statfs_t
	if syscall.Statfs("/tmp", &disk) == nil {
		m.DiskFree = disk.Bavail * uint64(disk.Bsize)
		m.DiskTotal = disk.Blocks * uint64(disk.Bsize)
	}
	stat, _ := os.ReadFile("/proc/stat")
	line := strings.SplitN(string(stat), "\n", 2)[0]
	fields := strings.Fields(line)
	var total, idle uint64
	for i := 1; i < len(fields) && i <= 8; i++ {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		total += v
		if i == 4 || i == 5 {
			idle += v
		}
	}
	if total > c.lastTotal && c.lastTotal > 0 {
		m.CPU = 100 * (1 - float64(idle-c.lastIdle)/float64(total-c.lastTotal))
	}
	c.lastTotal, c.lastIdle = total, idle
	filter := url.QueryEscape(`{"label":["cloudrail.managed=true"]}`)
	r, e := c.request(ctx, "GET", "/containers/json?all=true&filters="+filter, nil)
	if e != nil {
		return m
	}
	if r.StatusCode != 200 {
		r.Body.Close()
		return m
	}
	var containers []struct {
		ID     string
		State  string
		Status string
		Labels map[string]string
	}
	e = json.NewDecoder(r.Body).Decode(&containers)
	r.Body.Close()
	if e != nil {
		return m
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for _, v := range containers {
		if ctx.Err() != nil {
			break
		}
		if len(v.Labels["cloudrail.deployment"]) != 24 {
			continue
		}
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			value := ContainerMetric{Running: v.State == "running", Status: v.State}
			if strings.Contains(v.Status, "(unhealthy)") {
				value.Status = "unhealthy"
			}
			if value.Running {
				resp, err := c.request(ctx, "GET", "/containers/"+v.ID+"/stats?stream=false&one-shot=true", nil)
				if err == nil {
					var stats struct {
						CPUStats struct {
							CPUUsage struct {
								Total uint64 `json:"total_usage"`
							} `json:"cpu_usage"`
						} `json:"cpu_stats"`
						MemoryStats struct {
							Usage uint64
							Stats map[string]uint64
						} `json:"memory_stats"`
					}
					if resp.StatusCode == 200 && json.NewDecoder(resp.Body).Decode(&stats) == nil {
						now := time.Now()
						mu.Lock()
						old, exists := c.samples[v.Labels["cloudrail.deployment"]]
						if exists && stats.CPUStats.CPUUsage.Total >= old.CPU {
							elapsed := now.Sub(old.At)
							if elapsed > 0 {
								percent := 100 * float64(stats.CPUStats.CPUUsage.Total-old.CPU) / float64(elapsed)
								value.CPU = &percent
							}
						}
						c.samples[v.Labels["cloudrail.deployment"]] = cpuSample{CPU: stats.CPUStats.CPUUsage.Total, At: now}
						mu.Unlock()
						usage := stats.MemoryStats.Usage
						cache := stats.MemoryStats.Stats["inactive_file"]
						if cache == 0 {
							cache = stats.MemoryStats.Stats["total_inactive_file"]
						}
						if cache < usage {
							usage -= cache
						}
						value.Memory = &usage
					}
					resp.Body.Close()
				}
				if port := v.Labels["cloudrail.port"]; port != "" && v.Labels["cloudrail.kind"] == "http" {
					probe, err := http.NewRequestWithContext(ctx, "GET", "http://cloudrail-app-"+v.Labels["cloudrail.deployment"]+":"+port+v.Labels["cloudrail.health-path"], nil)
					if err == nil {
						client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
						response, err := client.Do(probe)
						if err != nil {
							value.Status = "unhealthy"
						} else {
							if response.StatusCode < 200 || response.StatusCode >= 300 {
								value.Status = "unhealthy"
							}
							response.Body.Close()
						}
					}
				}
			}
			mu.Lock()
			m.Containers[v.Labels["cloudrail.deployment"]] = value
			mu.Unlock()
		}()
	}
	wg.Wait()
	for id := range c.samples {
		if _, ok := m.Containers[id]; !ok {
			delete(c.samples, id)
		}
	}
	return m
}
