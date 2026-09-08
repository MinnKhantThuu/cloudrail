package docker

import (
	"archive/tar"
	"cloudrail/internal/deployment"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

func (c *Client) ExportVolume(ctx context.Context, d deployment.Deployment, out io.Writer) error {
	r, e := c.request(ctx, "GET", "/containers/"+deployment.Container(d.ID)+"/archive?path="+url.QueryEscape(d.Settings.MountPath), nil)
	if e != nil {
		return e
	}
	if r.StatusCode != 200 {
		return responseError(r)
	}
	defer r.Body.Close()
	n, e := io.Copy(out, io.LimitReader(r.Body, (1<<30)+1))
	if e != nil {
		return e
	}
	if n > 1<<30 {
		return errors.New("volume export exceeds the 1 GB beta limit")
	}
	return nil
}
func (c *Client) RestoreVolume(ctx context.Context, d deployment.Deployment, file, sourceMount string) error {
	if e := validateVolumeArchive(file, sourceMount); e != nil {
		return e
	}
	r, e := c.request(ctx, "GET", "/containers/"+deployment.Container(d.ID)+"/archive?path="+url.QueryEscape(d.Settings.MountPath), nil)
	if e != nil {
		return e
	}
	if r.StatusCode != 200 {
		return responseError(r)
	}
	tr := tar.NewReader(r.Body)
	count := 0
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			r.Body.Close()
			return e
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		count++
		if count > 1 || h.Typeflag != tar.TypeDir {
			r.Body.Close()
			return errors.New("restore requires an empty stopped volume")
		}
	}
	r.Body.Close()
	return c.putVolumeArchive(ctx, d, file, sourceMount)
}

func validateVolumeArchive(file, sourceMount string) error {
	// Validate every entry before allowing Docker to unpack into the target volume.
	validate := func(h *tar.Header) error {
		name := strings.TrimSuffix(h.Name, "/")
		root := path.Base(sourceMount)
		if name != root && !strings.HasPrefix(name, root+"/") {
			return errors.New("archive does not match source volume")
		}
		if path.Clean(name) != name || strings.ContainsAny(name, "\x00\\") || h.Typeflag != tar.TypeDir && h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return errors.New("volume restore rejects links and unsafe paths")
		}
		return nil
	}
	f, e := os.Open(file)
	if e != nil {
		return e
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if e = validate(h); e != nil {
			return e
		}
	}
	return nil
}

func (c *Client) putVolumeArchive(ctx context.Context, d deployment.Deployment, file, sourceMount string) error {
	f, e := os.Open(file)
	if e != nil {
		return e
	}
	defer f.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() {
		tr := tar.NewReader(f)
		tw := tar.NewWriter(writer)
		var err error
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				err = e
				break
			}
			h.Name = path.Base(d.Settings.MountPath) + strings.TrimPrefix(h.Name, path.Base(sourceMount))
			if err = tw.WriteHeader(h); err != nil {
				break
			}
			if _, err = io.Copy(tw, tr); err != nil {
				break
			}
		}
		if err == nil {
			err = tw.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	req, e := http.NewRequestWithContext(ctx, "PUT", "http://docker/v1.47/containers/"+deployment.Container(d.ID)+"/archive?path="+url.QueryEscape(path.Dir(d.Settings.MountPath)), reader)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/x-tar")
	resp, e := c.http.Do(req)
	if e != nil {
		return e
	}
	if resp.StatusCode != 200 {
		return responseError(resp)
	}
	resp.Body.Close()
	return nil
}

// RestoreRedisVolume replaces only a Redis volume that the agent has already
// proven logically empty with DBSIZE. A fresh Redis instance writes AOF
// metadata, so the generic filesystem-empty guard cannot handle that target.
func (c *Client) RestoreRedisVolume(ctx context.Context, d deployment.Deployment, file, sourceMount string) error {
	if e := validateVolumeArchive(file, sourceMount); e != nil {
		return e
	}
	if e := c.clearRedisVolume(ctx, d); e != nil {
		return e
	}
	return c.RestoreVolume(ctx, d, file, sourceMount)
}

func (c *Client) clearRedisVolume(ctx context.Context, d deployment.Deployment) error {
	body := map[string]any{
		"Image":      d.Image,
		"Entrypoint": []string{"/bin/sh", "-lc"},
		"Cmd":        []string{`find /target -mindepth 1 -maxdepth 1 -exec rm -rf -- {} \;`},
		"Labels":     map[string]string{"cloudrail.managed": "true", "cloudrail.restore": d.ID},
		"HostConfig": map[string]any{
			"NetworkMode": "none", "Memory": int64(64 << 20), "NanoCpus": int64(100_000_000), "PidsLimit": 32,
			"CapDrop": []string{"NET_RAW"}, "SecurityOpt": []string{"no-new-privileges:true"},
			"Mounts": []map[string]any{{"Type": "volume", "Source": d.Settings.VolumeName, "Target": "/target"}},
		},
	}
	resp, e := c.request(ctx, "POST", "/containers/create", body)
	if e != nil {
		return e
	}
	if resp.StatusCode != http.StatusCreated {
		return responseError(resp)
	}
	var created struct {
		ID string `json:"Id"`
	}
	e = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if e != nil {
		return e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if removed, err := c.request(cleanup, "DELETE", "/containers/"+url.PathEscape(created.ID)+"?force=true&v=false", nil); err == nil {
			removed.Body.Close()
		}
	}()
	resp, e = c.request(ctx, "POST", "/containers/"+url.PathEscape(created.ID)+"/start", nil)
	if e != nil {
		return e
	}
	if resp.StatusCode != http.StatusNoContent {
		return responseError(resp)
	}
	resp.Body.Close()
	resp, e = c.request(ctx, "POST", "/containers/"+url.PathEscape(created.ID)+"/wait?condition=not-running", nil)
	if e != nil {
		return e
	}
	if resp.StatusCode != http.StatusOK {
		return responseError(resp)
	}
	defer resp.Body.Close()
	var result struct{ StatusCode int }
	if e = json.NewDecoder(resp.Body).Decode(&result); e != nil {
		return e
	}
	if result.StatusCode != 0 {
		return fmt.Errorf("Redis volume preparation failed (exit %d)", result.StatusCode)
	}
	return nil
}
