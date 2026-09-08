package builds

import (
	"math"
	"testing"
)

func TestDiskReserve(t *testing.T) {
	dir := t.TempDir()
	if e := CheckDisk(dir, 0); e != nil {
		t.Fatal(e)
	}
	if CheckDisk(dir, math.MaxUint64) == nil {
		t.Fatal("disk pressure did not block work")
	}
}
