//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func powershell() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}
func hideCommand(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func launchUI(exe, dir string) error {
	p := powershell()
	if _, err := os.Stat(p); err != nil {
		return fmt.Errorf("Windows PowerShell 5.1 is required: %w", err)
	}
	c := exec.Command(p, "-NoLogo", "-NoProfile", "-STA", "-ExecutionPolicy", "RemoteSigned", "-File", filepath.Join(dir, "ui.ps1"), "-Engine", exe, "-Assets", dir)
	hideCommand(c)
	f, err := os.OpenFile(filepath.Join(dir, "startup.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err == nil {
		defer f.Close()
		c.Stdout = f
		c.Stderr = f
	}
	return c.Run()
}
func showError(s string) {
	dll := syscall.NewLazyDLL("user32.dll")
	p := dll.NewProc("MessageBoxW")
	text, _ := syscall.UTF16PtrFromString(s)
	title, _ := syscall.UTF16PtrFromString("ApplyKit")
	p.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
