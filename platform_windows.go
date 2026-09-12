//go:build windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func powershell() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}
func hideCommand(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

var desktopRenderer PDFRenderFunc = renderPDF

func rendererName() string { return "Windows.Data.Pdf" }

func browserCandidates() []string {
	var out []string
	pf := os.Getenv("ProgramFiles")
	pfx := os.Getenv("ProgramFiles(x86)")
	local := os.Getenv("LOCALAPPDATA")
	for _, p := range []string{
		filepath.Join(pf, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(pfx, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(local, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(pf, "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(pfx, "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe"),
	} {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
func findBrowser() string {
	for _, p := range browserCandidates() {
		if st, e := os.Stat(p); e == nil && !st.IsDir() {
			return p
		}
	}
	for _, n := range []string{"msedge.exe", "chrome.exe"} {
		if p, e := exec.LookPath(n); e == nil {
			return p
		}
	}
	return ""
}
func launchAppWindow(url string) error {
	if browser := findBrowser(); browser != "" {
		c := exec.Command(browser, "--app="+url, "--new-window", "--window-size=1500,950", "--no-first-run")
		hideCommand(c)
		if err := c.Start(); err == nil {
			return nil
		}
	}
	c := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", url)
	hideCommand(c)
	return c.Start()
}
func chooseFolder(ctx context.Context, current, assets string) (string, error) {
	ps := powershell()
	if _, err := os.Stat(ps); err != nil {
		return "", fmt.Errorf("Windows PowerShell 5.1 is unavailable")
	}
	script := `Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='选择 ApplyKit 输出文件夹'; if($env:APPLYKIT_FOLDER -and (Test-Path -LiteralPath $env:APPLYKIT_FOLDER)){ $d.SelectedPath=$env:APPLYKIT_FOLDER }; if($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){[Console]::OutputEncoding=[Text.UTF8Encoding]::UTF8; Write-Output $d.SelectedPath}else{exit 3}`
	c := exec.CommandContext(ctx, ps, "-NoLogo", "-NoProfile", "-STA", "-ExecutionPolicy", "RemoteSigned", "-Command", script)
	c.Env = append(os.Environ(), "APPLYKIT_FOLDER="+current)
	hideCommand(c)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 3 {
			return "", fmt.Errorf("已取消选择")
		}
		return "", fmt.Errorf("无法选择文件夹：%s", strings.TrimSpace(stderr.String()))
	}
	p := strings.TrimSpace(strings.TrimPrefix(stdout.String(), "\ufeff"))
	if p == "" || !filepath.IsAbs(p) {
		return "", fmt.Errorf("没有选择有效文件夹")
	}
	return p, nil
}
func openLocalPath(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	var c *exec.Cmd
	if st.IsDir() {
		c = exec.Command("explorer.exe", path)
	} else {
		c = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", path)
	}
	hideCommand(c)
	return c.Start()
}
func runtimeChecks() map[string]any {
	_, psErr := os.Stat(powershell())
	b := findBrowser()
	return map[string]any{
		"powerShell51":   psErr == nil,
		"browserAppMode": b != "",
		"browser":        filepath.Base(b),
		"pdfRenderer":    "Windows.Data.Pdf",
	}
}
func showError(s string) {
	dll := syscall.NewLazyDLL("user32.dll")
	p := dll.NewProc("MessageBoxW")
	text, _ := syscall.UTF16PtrFromString(s)
	title, _ := syscall.UTF16PtrFromString("ApplyKit")
	p.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
