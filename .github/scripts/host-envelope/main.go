// CI-only envelope for synthetic host-recovery fixtures. Never handles production backups.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

var header = []byte("CLOUDRAIL-CI-BACKUP-1\n")

const limit = 512 << 20

func transform(mode string, key, data []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("CI transfer requires a 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	if mode == "seal" {
		if len(data) > limit {
			return nil, errors.New("CI fixture exceeds 512 MiB")
		}
		return append(append([]byte{}, header...), gcm.Seal(nil, nil, data, header)...), nil
	}
	if mode != "open" || !bytes.HasPrefix(data, header) {
		return nil, errors.New("unsupported CI transfer envelope")
	}
	return gcm.Open(nil, nil, data[len(header):], header)
}

func run() error {
	if len(os.Args) != 4 {
		return errors.New("usage: host-envelope seal|open INPUT OUTPUT")
	}
	key, err := hex.DecodeString(os.Getenv("CLOUDRAIL_RECOVERY_TEST_KEY"))
	if err != nil {
		return errors.New("invalid CI transfer key")
	}
	stat, err := os.Stat(os.Args[2])
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() > limit+1024 {
		return errors.New("invalid or oversized CI fixture")
	}
	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		return err
	}
	result, err := transform(os.Args[1], key, data)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(os.Args[3], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = output.Write(result)
	closeErr := output.Close()
	if err != nil {
		_ = os.Remove(os.Args[3])
		return err
	}
	return closeErr
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "CI transfer refused:", err)
		os.Exit(1)
	}
	fmt.Println("PASS authenticated CI transfer envelope")
}
