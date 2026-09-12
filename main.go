package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const version = "2.2.0"

//go:embed assets/*
var assets embed.FS

func assetDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	entries, err := assets.ReadDir("assets")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, er := assets.ReadFile("assets/" + e.Name())
		if er != nil {
			return "", er
		}
		h.Write([]byte(e.Name()))
		h.Write(b)
	}
	dir := filepath.Join(base, "ApplyKit", version+"-"+hex.EncodeToString(h.Sum(nil))[:10])
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, er := assets.ReadFile("assets/" + e.Name())
		if er != nil {
			return "", er
		}
		p := filepath.Join(dir, e.Name())
		old, _ := os.ReadFile(p)
		if bytes.Equal(old, b) {
			continue
		}
		tmp, er := os.CreateTemp(dir, ".asset-*")
		if er != nil {
			return "", er
		}
		name := tmp.Name()
		_, er = tmp.Write(b)
		ce := tmp.Close()
		if er == nil {
			er = ce
		}
		if er != nil {
			_ = os.Remove(name)
			return "", er
		}
		// Content-addressed version directories are immutable after extraction.
		_ = os.Remove(p)
		if er = os.Rename(name, p); er != nil {
			_ = os.Remove(name)
			return "", er
		}
	}
	return dir, nil
}

func workerMain(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 2
	}
	_ = os.Remove(path)
	var job Job
	if err = json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), &job); err != nil {
		return 2
	}
	dir, err := assetDir()
	if err != nil {
		return 2
	}
	return runJob(job, dir)
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("ApplyKit " + version)
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--assets-info" {
		dir, err := assetDir()
		if err != nil {
			os.Exit(2)
		}
		data, _ := json.Marshal(map[string]string{"version": version, "directory": dir})
		if err = os.WriteFile(os.Args[2], data, 0600); err != nil {
			os.Exit(2)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--crop-preview" {
		os.Exit(cropPreviewMain(os.Args[2]))
	}
	if len(os.Args) == 3 && os.Args[1] == "--worker" {
		os.Exit(workerMain(os.Args[2]))
	}

	fs := flag.NewFlagSet("ApplyKit", flag.ContinueOnError)
	serve := fs.Bool("serve", false, "run local application server")
	port := fs.Int("port", 0, "local server port; 0 chooses an available port")
	ready := fs.String("ready", "", "write startup information JSON here")
	data := fs.String("data", "", "application data directory")
	noOpen := fs.Bool("no-open", false, "do not launch the application window")
	_ = fs.Parse(os.Args[1:])

	ctx := context.Background()
	openWindow := true
	if *serve {
		openWindow = !*noOpen
	}
	if err := serveApplication(ctx, *port, *ready, *data, openWindow); err != nil {
		showError("ApplyKit could not start.\n\n" + err.Error() + "\n\nSee README.md / 使用说明.html for requirements.")
		os.Exit(1)
	}
}
