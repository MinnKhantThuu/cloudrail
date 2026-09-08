package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cloudrail/internal/deployment"
)

type Routes struct {
	Directory string
	HTTPS     bool
}

func (r Routes) Set(service deployment.Service, d *deployment.Deployment) error {
	if err := os.MkdirAll(r.Directory, 0700); err != nil {
		return err
	}
	// Traefik watches YAML/TOML extensions. JSON is valid YAML, but .json is ignored.
	path := filepath.Join(r.Directory, service.ID+".yaml")
	if d == nil || d.Settings.Kind == "postgres" {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	key := "svc-" + service.ID
	body := map[string]any{"http": map[string]any{
		"routers":     map[string]any{key: map[string]any{"rule": "Host(`" + service.Host + "`)", "entryPoints": []string{"web"}, "service": key, "middlewares": []string{key}}},
		"services":    map[string]any{key: map[string]any{"loadBalancer": map[string]any{"servers": []map[string]string{{"url": fmt.Sprintf("http://%s:%d", deployment.Container(d.ID), d.Port)}}}}},
		"middlewares": map[string]any{key: map[string]any{"headers": map[string]any{"customResponseHeaders": map[string]string{"X-Cloudrail-Deployment": d.ID}}}},
	}}
	if r.HTTPS {
		routers := body["http"].(map[string]any)["routers"].(map[string]any)
		secure := routers[key].(map[string]any)
		secure["entryPoints"] = []string{"websecure"}
		secure["tls"] = map[string]string{"certResolver": "letsencrypt"}
		routers[key+"-probe"] = map[string]any{"rule": "Host(`" + service.Host + "`)", "entryPoints": []string{"probe"}, "service": key, "middlewares": []string{key}}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if old, e := os.ReadFile(path); e == nil && bytes.Equal(old, data) {
		return nil
	}
	f, err := os.CreateTemp(r.Directory, ".route-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(r.Directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
