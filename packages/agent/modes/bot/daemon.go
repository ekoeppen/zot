package bot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WritePIDFile persists pid to path. Overwrites any existing file.
func WritePIDFile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// ReadPIDFile returns the pid stored at path, or 0 if the file doesn't exist.
func ReadPIDFile(path string) (int, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, fmt.Errorf("parse pid: %w", err)
	}
	return pid, nil
}

// RemovePIDFile deletes the pid file if it exists.
func RemovePIDFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsRunningAt reports whether a live process is recorded at pidPath.
// Semantics identical to the old telegram.IsRunning.
func IsRunningAt(pidPath string) (int, bool, error) {
	pid, err := ReadPIDFile(pidPath)
	if err != nil {
		return 0, false, err
	}
	if pid <= 0 {
		return 0, false, nil
	}
	alive, err := processAlive(pid)
	if err != nil {
		return pid, false, nil
	}
	return pid, alive, nil
}

// StopProcess asks pid to exit, waits up to graceful, then force-kills.
func StopProcess(pid int, graceful time.Duration) error {
	return stopProcess(pid, graceful)
}
