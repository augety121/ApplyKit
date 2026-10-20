package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanSizeBudgets(t *testing.T) {
	for _, size := range [][2]int{{4961, 7016}, {7016, 9933}, {9922, 14032}, {16000, 10000}, {160000000, 1}} {
		if err := validateImageSize(image.Config{Width: size[0], Height: size[1], ColorModel: color.YCbCrModel}); err != nil {
			t.Fatal(err)
		}
		w, h := workingSize(size[0], size[1], 0)
		if int64(w)*int64(h) > maxPixels || w > size[0] || h > size[1] {
			t.Fatalf("bad working dimensions %d x %d", w, h)
		}
	}
	for _, cfg := range []image.Config{{Width: 0, Height: 1}, {Width: 16001, Height: 10000}, {Width: 10000, Height: 10000, ColorModel: color.NRGBA64Model}, {Width: int(^uint(0) >> 1), Height: 2}} {
		if validateImageSize(cfg) == nil {
			t.Fatalf("accepted oversized configuration: %+v", cfg)
		}
	}
}

// A synthetic 35 MP scan exercises the same upload/preview/export paths as a
// printer scan; no personal document or decoder-header-only fixture is used.
func TestLargeScanUploadPreviewAndExport(t *testing.T) {
	im := image.NewGray(image.Rect(0, 0, 5000, 7000))
	for i := range im.Pix {
		im.Pix[i] = 255
	}
	for y := 700; y < 6300; y++ {
		if y%120 < 18 {
			for x := 600; x < 4400; x++ {
				im.Pix[y*im.Stride+x] = 30
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, im, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), buf.Bytes()...)
	s := testApp(t)
	req := httptest.NewRequest("POST", "http://localhost/api/upload?name=synthetic-scan.jpg", bytes.NewReader(original))
	req.Header.Set("Authorization", "Bearer "+s.token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var upload struct {
		File FileItem `json:"file"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	item, err := s.file(upload.File.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Width != 5000 || item.Height != 7000 || item.ProcessingNote == "" {
		t.Fatalf("missing original dimensions or notice: %+v", item)
	}
	p, err := s.preparePreview(context.Background(), item.ID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if max(p.Width, p.Height) > 1600 || p.SourceWidth != 5000 || p.SourceHeight != 7000 {
		t.Fatalf("bad preview: %+v", p)
	}
	src, err := loadImage(item.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !src.Reduced || int64(src.Image.Bounds().Dx())*int64(src.Image.Bounds().Dy()) > maxPixels {
		t.Fatal("scan was not reduced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = decodeImageSized(ctx, original, 1600); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	rect := NormalizedRect{Left: 0.1, Top: 0.1, Right: 0.9, Bottom: 0.9}
	job := Job{Format: "jpg", Limit: 500000, Profile: "balanced", MaxEdge: 2400, Edits: map[string]ImageEdit{item.Path: {SHA256: item.SHA256, Rotation: 90, FlipH: true, Rect: &rect}}}
	reporter := &Reporter{}
	if err = processImage(context.Background(), job, item.Path, t.TempDir(), reporter); err != nil {
		t.Fatal(err)
	}
	if len(reporter.Records) != 1 {
		t.Fatal("missing result")
	}
	rec := reporter.Records[0]
	if rec.Status != "saved" || rec.After >= 500000 || max(rec.Width, rec.Height) > 2400 || !strings.Contains(rec.Message, "扫描图") {
		t.Fatalf("bad export: %+v", rec)
	}
	output, err := os.ReadFile(rec.Output)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(output))
	if err != nil || cfg.Width != rec.Width || cfg.Height != rec.Height {
		t.Fatal("export dimensions do not match actual JPEG")
	}
	stored, err := os.ReadFile(item.Path)
	if err != nil || !bytes.Equal(stored, original) {
		t.Fatal("original scan changed")
	}
}

func TestThumbnailCompositesTransparencyAndKeepsSourceDimensions(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 40, 60))
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "transparent.png")
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	src, err := loadImageSized(context.Background(), path, 30)
	if err != nil {
		t.Fatal(err)
	}
	if src.SourceWidth != 40 || src.SourceHeight != 60 || src.Image.Bounds().Dx() != 20 || src.Image.Bounds().Dy() != 30 {
		t.Fatal("thumbnail dimensions")
	}
	if src.Image.RGBAAt(5, 5) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("transparent scan must composite on white")
	}
}
