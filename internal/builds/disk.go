package builds

import (
	"fmt"
	"syscall"
)

func CheckDisk(path string, minimum uint64) error {
	var stat syscall.Statfs_t
	if e := syscall.Statfs(path, &stat); e != nil {
		return e
	}
	free := stat.Bavail * uint64(stat.Bsize)
	if free < minimum {
		return fmt.Errorf("not enough disk space: %d MB free; %d MB required", free>>20, minimum>>20)
	}
	return nil
}
