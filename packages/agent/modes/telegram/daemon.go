package telegram

import (
	"path/filepath"
	"time"

	"github.com/patriceckhart/zot/packages/agent/modes/bot"
)

// PIDPath returns the location of the bot's pid file.
func PIDPath(zotHome string) string { return filepath.Join(zotHome, "bot.pid") }

// LogPath returns the location of the bot's log file (stdout+stderr
// from a detached `zot bot start`).
func LogPath(zotHome string) string { return filepath.Join(zotHome, "logs", "bot.log") }

// WritePID persists pid to bot.pid. Overwrites any existing file.
func WritePID(zotHome string, pid int) error { return bot.WritePIDFile(PIDPath(zotHome), pid) }

// ReadPID returns the pid stored in bot.pid, or 0 if the file doesn't
// exist. Returns an error for any other read/parse failure.
func ReadPID(zotHome string) (int, error) { return bot.ReadPIDFile(PIDPath(zotHome)) }

// RemovePID deletes the pid file if it exists.
func RemovePID(zotHome string) error { return bot.RemovePIDFile(PIDPath(zotHome)) }

// IsRunning returns (pid, true) if a live process with the recorded
// pid exists, or (pid, false) if the pid file points to a dead process.
// Stale pid files are left in place; the caller may remove them.
func IsRunning(zotHome string) (int, bool, error) { return bot.IsRunningAt(PIDPath(zotHome)) }

// StopProcess asks pid to exit and waits up to graceful for it to stop,
// then escalates to a forced kill. Returns nil if the process is gone.
func StopProcess(pid int, graceful time.Duration) error { return bot.StopProcess(pid, graceful) }
