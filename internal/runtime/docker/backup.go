package docker

import (
	"archive/tar"
	"cloudrail/internal/deployment"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"os"
)

func copyDockerOutput(stdout, stderr io.Writer, src io.Reader) error {
	var header [8]byte
	for {
		_, e := io.ReadFull(src, header[:])
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		n := binary.BigEndian.Uint32(header[4:])
		if n > 8<<20 {
			return errors.New("Docker output frame too large")
		}
		out := io.Discard
		if header[0] == 1 {
			out = stdout
		} else if header[0] == 2 {
			out = stderr
		}
		if _, e = io.CopyN(out, src, int64(n)); e != nil {
			return e
		}
	}
}
func (c *Client) PutRestore(ctx context.Context, id, file string) error {
	f, e := os.Open(file)
	if e != nil {
		return e
	}
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return e
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() {
		defer f.Close()
		tw := tar.NewWriter(writer)
		err := tw.WriteHeader(&tar.Header{Name: "cloudrail-restore.dump", Mode: 0600, Size: info.Size()})
		if err == nil {
			_, err = io.Copy(tw, f)
		}
		if err == nil {
			err = tw.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	req, e := http.NewRequestWithContext(ctx, "PUT", "http://docker/v1.47/containers/"+deployment.Container(id)+"/archive?path=/tmp", reader)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/x-tar")
	r, e := c.http.Do(req)
	if e != nil {
		return e
	}
	if r.StatusCode != 200 {
		return responseError(r)
	}
	r.Body.Close()
	return nil
}
