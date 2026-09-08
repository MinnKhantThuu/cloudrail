package builds

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func archiveFixture(h *tar.Header, body string) []byte {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(h)
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}
func TestExtractionBoundaries(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "root/../../escape", Size: 1, Mode: 0600}, {Name: "root/link", Typeflag: tar.TypeSymlink, Linkname: "../../etc"}, {Name: "root/device", Typeflag: tar.TypeChar}, {Name: "root/huge", Size: 51 << 20}} {
		if e := Extract(bytes.NewReader(archiveFixture(h, "x")), t.TempDir()); e == nil {
			t.Fatalf("accepted unsafe entry: %s", h.Name)
		}
	}
	dir := t.TempDir()
	if e := Extract(bytes.NewReader(archiveFixture(&tar.Header{Name: "root/app.js", Size: 5, Mode: 0644}, "hello")), dir); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "app.js"))
	if e != nil || string(b) != "hello" {
		t.Fatal("source content mismatch")
	}
}
func TestSourceValidation(t *testing.T) {
	c := Config{Repository: "owner/repo", Branch: "main", Builder: "dockerfile", Port: 80, HealthPath: "/"}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.Root = "../credentials"
	if c.Validate() == nil {
		t.Fatal("root escaped")
	}
	tail := &Tail{}
	_, _ = tail.Write(bytes.Repeat([]byte("x"), 100000))
	if len(tail.String()) > 15000 {
		t.Fatal("unbounded logs")
	}
}
