package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const version = "1.1.0"

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
		b, _ := assets.ReadFile("assets/" + e.Name())
		h.Write(b)
	}
	dir := filepath.Join(base, "ApplyKit", version+"-"+hex.EncodeToString(h.Sum(nil))[:10])
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	for _, e := range entries {
		b, er := assets.ReadFile("assets/" + e.Name())
		if er != nil {
			return "", er
		}
		p := filepath.Join(dir, e.Name())
		old, _ := os.ReadFile(p)
		if !bytes.Equal(old, b) {
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
				os.Remove(name)
				return "", er
			}
			// Existing assets are immutable for a given content-addressed version.
			if er = os.Rename(name, p); er != nil {
				os.Remove(name)
				return "", er
			}
		}
	}
	return dir, nil
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--assets-info" {
		dir, err := assetDir()
		if err != nil {
			os.Exit(2)
		}
		data, err := json.Marshal(map[string]string{"version": version, "directory": dir})
		if err != nil {
			os.Exit(2)
		}
		if err = os.WriteFile(os.Args[2], data, 0600); err != nil {
			os.Exit(2)
		}
		return
	}

	if len(os.Args) == 3 && os.Args[1] == "--crop-preview" {
		os.Exit(cropPreviewMain(os.Args[2]))
	}
	if len(os.Args) == 3 && os.Args[1] == "--worker" {
		code := workerMain(os.Args[2])
		os.Exit(code)
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("ApplyKit " + version)
		return
	}
	exe, err := os.Executable()
	if err == nil {
		var dir string
		dir, err = assetDir()
		if err == nil {
			err = launchUI(exe, dir)
		}
	}
	if err != nil {
		showError("ApplyKit could not start.\n\n" + err.Error() + "\n\nSee the included README for requirements.")
	}
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
