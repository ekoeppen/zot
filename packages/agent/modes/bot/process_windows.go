//go:build windows

package bot

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const stillActive = 259

func processAlive(pid int) (bool, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return true, nil
		}
		return false, err
	}
	defer windows.CloseHandle(h)

	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false, err
	}
	return code == stillActive, nil
}

func stopProcess(pid int, graceful time.Duration) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = proc.Signal(os.Interrupt)

	deadline := time.Now().Add(graceful)
	for time.Now().Before(deadline) {
		alive, err := processAlive(pid)
		if err != nil || !alive {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = proc.Kill()
	return nil
}
