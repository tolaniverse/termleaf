//go:build !unix

package render

import "os/exec"

func isolateProcess(_ *exec.Cmd) {}

func killProcessTree(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}
