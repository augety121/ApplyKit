//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func powershell() string      { return "powershell.exe" }
func hideCommand(c *exec.Cmd) {}
func launchUI(exe, dir string) error {
	return fmt.Errorf("the graphical application requires Windows 10/11 x64")
}
func showError(s string) { fmt.Fprintln(os.Stderr, s) }
