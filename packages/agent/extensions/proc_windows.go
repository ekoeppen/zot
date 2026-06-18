//go:build windows

package extensions

import "os/exec"

func isolateExtensionProcess(_ *exec.Cmd) {}
