package main

import (
	"context"
	"image"
	"image/color"
	"testing"
)

func TestParseByteLimitCustomValues(t *testing.T) {
	cases := []struct {
		amount, unit string
		want         int64
	}{
		{"200", "KB", 200000},
		{"500", "KB", 500000},
		{"1.5", "MB", 1500000},
		{"2", "MB", 2000000},
		{"0.001", "MB", 1000},
		{"100", "MB", 100000000},
	}
	for _, tc := range cases {
		got, err := parseByteLimit(tc.amount, tc.unit)
		if err != nil || got != tc.want {
			t.Fatalf("%s %s => %d,%v; want %d", tc.amount, tc.unit, got, err, tc.want)
		}
	}
	for _, tc := range [][2]string{{"0", "MB"}, {"100.1", "MB"}, {"1e2", "KB"}, {"-1", "MB"}, {"abc", "KB"}} {
		if _, err := parseByteLimit(tc[0], tc[1]); err == nil {
			t.Fatalf("expected invalid: %v", tc)
		}
	}
}

func TestImageEditCropRotateAndMaxEdge(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			src.SetRGBA(x, y, color.RGBA{245, 245, 245, 255})
		}
	}
	edit := ImageEdit{Rotation: 90, Rect: &NormalizedRect{Left: .25, Top: .25, Right: .75, Bottom: .75}}
	out, info, err := applyImageEdit(context.Background(), src, edit, 100)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || !info.Applied {
		t.Fatalf("expected applied crop: %+v", info)
	}
	if max(out.Bounds().Dx(), out.Bounds().Dy()) > 100 {
		t.Fatalf("max edge not enforced: %v", out.Bounds())
	}
}

func TestAutoTrimKeepsSparseContentConservatively(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			src.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	src.SetRGBA(150, 150, color.RGBA{0, 0, 0, 255})
	det, err := detectWhiteMargins(context.Background(), src, 250, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if det.Bounds != src.Bounds() {
		t.Fatalf("sparse mark should preserve full page, got %v", det.Bounds)
	}
}
