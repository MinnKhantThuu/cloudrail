package docker

import (
	"archive/tar"
	"cloudrail/internal/deployment"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
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
	tr = tar.NewReader(f)
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
	if _, e = f.Seek(0, 0); e != nil {
		return e
	}
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
