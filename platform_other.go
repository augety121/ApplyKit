//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func powershell() string      { return "powershell.exe" }
func hideCommand(c *exec.Cmd) {}

var desktopRenderer PDFRenderFunc = renderPDF

func rendererName() string { return "Windows.Data.Pdf (Windows only)" }
func launchAppWindow(url string) error {
	for _, n := range []string{"chromium", "google-chrome", "chrome"} {
		if p, e := exec.LookPath(n); e == nil {
			return exec.Command(p, "--app="+url, "--window-size=1500,950").Start()
		}
	}
	if p, e := exec.LookPath("xdg-open"); e == nil {
		return exec.Command(p, url).Start()
	}
	return fmt.Errorf("no supported browser launcher found")
}
func chooseFolder(ctx context.Context, current, assets string) (string, error) {
	if current != "" && filepath.IsAbs(current) {
		return current, nil
	}
	return "", fmt.Errorf("folder picker is available in the Windows build")
}
func openLocalPath(path string) error {
	p, e := exec.LookPath("xdg-open")
	if e != nil {
		return e
	}
	return exec.Command(p, path).Start()
}
func runtimeChecks() map[string]any {
	return map[string]any{"powerShell51": false, "browserAppMode": true, "pdfRenderer": "Windows only"}
}
func showError(s string) { fmt.Fprintln(os.Stderr, s) }
