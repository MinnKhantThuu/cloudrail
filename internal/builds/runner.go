package builds

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Tail struct {
	mu   sync.Mutex
	text string
}

func (t *Tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.text += string(b)
	if len(t.text) > 15000 {
		t.text = t.text[len(t.text)-15000:]
	}
	return len(b), nil
}
func (t *Tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.ToValidUTF8(t.text, "")
}

// Extract strips GitHub's one enclosing directory. Links and special files are not accepted.
func Extract(reader io.Reader, dest string) error {
	gz, e := gzip.NewReader(io.LimitReader(reader, 100<<20))
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	count := 0
	top := ""
	for {
		h, e := tr.Next()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		count++
		if count > 20000 {
			return errors.New("source exceeds 20000 files")
		}
		name := strings.TrimSuffix(h.Name, "/")
		if !safePath(name) {
			return errors.New("unsafe archive path")
		}
		parts := strings.SplitN(name, "/", 2)
		if top == "" {
			top = parts[0]
		}
		if top != parts[0] {
			return errors.New("archive has multiple roots")
		}
		if len(parts) == 1 {
			continue
		}
		name = parts[1]
		if !safePath(name) {
			return errors.New("unsafe archive path")
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if e = os.MkdirAll(target, 0700); e != nil {
				return e
			}
		case tar.TypeReg, tar.TypeRegA:
			if h.Size < 0 || h.Size > 50<<20 {
				return errors.New("source file exceeds 50 MB")
			}
			total += h.Size
			if total > 250<<20 {
				return errors.New("source exceeds 250 MB")
			}
			if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
				return e
			}
			f, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600|os.FileMode(h.Mode)&0111)
			if e != nil {
				return e
			}
			_, e = io.CopyN(f, tr, h.Size)
			closeErr := f.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("source links and special files are not supported")
		}
	}
}
func command(ctx context.Context, dir string, env []string, logs io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = logs
	cmd.Stderr = logs
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd.Run()
}
func Run(ctx context.Context, b Build, archive io.Reader, logs *Tail) (string, error) {
	if e := CheckDisk(os.TempDir(), 2<<30); e != nil {
		return "", e
	}
	dir, e := os.MkdirTemp("", "cloudrail-build-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(dir)
	source := filepath.Join(dir, "source")
	if e = os.MkdirAll(source, 0700); e != nil {
		return "", e
	}
	fmt.Fprintln(logs, "Fetching commit", b.Commit)
	if e = Extract(archive, source); e != nil {
		return "", e
	}
	root := filepath.Join(source, b.Config.Root)
	info, e := os.Stat(root)
	if e != nil || !info.IsDir() {
		return "", errors.New("configured source root does not exist")
	}
	env := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + filepath.Join(dir, "home"), "NO_COLOR=1"}
	_ = os.MkdirAll(filepath.Join(dir, "home"), 0700)
	tag := "127.0.0.1:5001/cloudrail/" + b.ServiceID + ":" + b.ID
	metadata := filepath.Join(dir, "metadata.json")
	addr := os.Getenv("BUILDKIT_HOST")
	if addr == "" {
		addr = "unix:///buildkit/buildkitd.sock"
	}
	args := []string{"--addr", addr, "build", "--progress=plain", "--local", "context=" + root, "--output", "type=image,name=" + tag + ",push=true,registry.insecure=true", "--metadata-file", metadata}
	if b.Config.Builder == "railpack" {
		plan := filepath.Join(dir, "plan")
		_ = os.MkdirAll(plan, 0700)
		if b.Config.BuildCommand != "" {
			env = append(env, "RAILPACK_BUILD_CMD="+b.Config.BuildCommand)
		}
		if b.Config.StartCommand != "" {
			env = append(env, "RAILPACK_START_CMD="+b.Config.StartCommand)
		}
		if e = command(ctx, root, env, logs, "railpack", "prepare", root, "--plan-out", filepath.Join(plan, "railpack-plan.json"), "--info-out", filepath.Join(plan, "railpack-info.json")); e != nil {
			return "", fmt.Errorf("Railpack prepare failed: %w", e)
		}
		args = append(args, "--frontend=gateway.v0", "--opt", "source=ghcr.io/railwayapp/railpack-frontend:v0.39.0", "--local", "dockerfile="+plan, "--opt", "build-arg:cache-key="+b.ServiceID)
	} else {
		file := b.Config.Dockerfile
		if file == "" {
			file = "Dockerfile"
		}
		full := filepath.Join(root, file)
		if _, e = os.Stat(full); e != nil {
			return "", errors.New("Dockerfile not found in configured root")
		}
		args = append(args, "--frontend=dockerfile.v0", "--local", "dockerfile="+filepath.Dir(full), "--opt", "filename="+filepath.Base(full))
	}
	fmt.Fprintln(logs, "Building and publishing image")
	if e = command(ctx, root, env, logs, "buildctl", args...); e != nil {
		return "", fmt.Errorf("BuildKit failed: %w", e)
	}
	raw, e := os.ReadFile(metadata)
	if e != nil {
		return "", e
	}
	var meta map[string]json.RawMessage
	if e = json.Unmarshal(raw, &meta); e != nil {
		return "", e
	}
	var digest string
	if e = json.Unmarshal(meta["containerimage.digest"], &digest); e != nil || !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
		return "", errors.New("builder returned no valid image digest")
	}
	return "127.0.0.1:5001/cloudrail/" + b.ServiceID + "@" + digest, nil
}
