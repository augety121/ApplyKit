package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type CropPreviewRequest struct {
	Input     string  `json:"input"`
	Output    string  `json:"output"`
	Progress  string  `json:"progress"`
	Cancel    string  `json:"cancel"`
	Threshold int     `json:"threshold"`
	MarginMM  float64 `json:"marginMM"`
}
type CropPreviewEvent struct {
	Kind    string          `json:"kind"`
	Message string          `json:"message,omitempty"`
	SHA256  string          `json:"sha256,omitempty"`
	Page    int             `json:"page,omitempty"`
	Total   int             `json:"total,omitempty"`
	Path    string          `json:"path,omitempty"`
	Width   int             `json:"width,omitempty"`
	Height  int             `json:"height,omitempty"`
	Rect    *NormalizedRect `json:"rect,omitempty"`
	Blank   bool            `json:"blank,omitempty"`
}

func cancelOnFile(ctx context.Context, cancel context.CancelFunc, path string) {
	if path == "" {
		return
	}
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, e := os.Stat(path); e == nil {
				cancel()
				return
			}
		}
	}
}
func cropPreviewMain(requestPath string) int {
	data, e := os.ReadFile(requestPath)
	if e != nil {
		return 2
	}
	_ = os.Remove(requestPath)
	var request CropPreviewRequest
	if e = json.Unmarshal(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), &request); e != nil {
		return 2
	}
	f, e := os.OpenFile(request.Progress, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return 2
	}
	defer f.Close()
	emit := func(ev CropPreviewEvent) { _ = json.NewEncoder(f).Encode(ev) }
	assets, e := assetDir()
	if e != nil {
		emit(CropPreviewEvent{Kind: "fatal", Message: e.Error()})
		return 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cancelOnFile(ctx, cancel, request.Cancel)
	if e = prepareCropPreview(ctx, request, assets, emit, renderPDF); e != nil {
		emit(CropPreviewEvent{Kind: "fatal", Message: e.Error()})
		return 1
	}
	emit(CropPreviewEvent{Kind: "complete", Message: "预览准备完成。框内内容会保留；保存后仍会按导出 DPI 重新渲染。"})
	return 0
}
func prepareCropPreview(ctx context.Context, req CropPreviewRequest, assets string, emit func(CropPreviewEvent), render PDFRenderFunc) error {
	c := CropOptions{Mode: "auto", Threshold: req.Threshold, MarginMM: req.MarginMM}
	if e := c.validate(); e != nil {
		return e
	}
	if !filepath.IsAbs(req.Output) {
		return fmt.Errorf("预览缓存目录必须是绝对路径")
	}
	if e := os.MkdirAll(req.Output, 0700); e != nil {
		return e
	}
	hash, e := fileSHA256(ctx, req.Input)
	if e != nil {
		return e
	}
	// Previews are deliberately bounded. Final export never reuses these pixels.
	job := Job{DPI: 110, RenderMaxLong: 1600, Cancel: req.Cancel, Crop: c}
	e = render(ctx, job, req.Input, assets, func(ev renderEvent) error {
		if ev.Kind == "meta" {
			emit(CropPreviewEvent{Kind: "meta", Total: ev.Total, SHA256: hash})
			return nil
		}
		if ev.Kind != "page" {
			return nil
		}
		defer os.Remove(ev.Path)
		src, er := loadImage(ev.Path)
		if er != nil {
			return er
		}
		mx, my := marginPixels(c, ev, src.Image.Bounds(), job.DPI)
		det, er := detectWhiteMargins(ctx, src.Image, c.threshold(), mx, my)
		if er != nil {
			return er
		}
		rect := normalizeRect(det.Bounds, src.Image.Bounds())
		path, er := saveUnique(req.Output, fmt.Sprintf("preview-%04d.png", ev.Page), src.Bytes)
		if er != nil {
			return er
		}
		emit(CropPreviewEvent{Kind: "page", Page: ev.Page, Path: path, Width: src.Image.Bounds().Dx(), Height: src.Image.Bounds().Dy(), Rect: &rect, Blank: det.Blank, Message: det.Note})
		return nil
	})
	if e != nil {
		return e
	}
	endHash, e := fileSHA256(ctx, req.Input)
	if e != nil {
		return e
	}
	if endHash != hash {
		return fmt.Errorf("预览过程中 PDF 已更改，请重新打开裁剪预览")
	}
	return nil
}
