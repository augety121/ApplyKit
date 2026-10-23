package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmallImageLimitsAndAutoFormat(t *testing.T) {
	src := sample(900, 1500, true)
	pngData, err := encodeImage(src, "png", 92)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "detailed-screenshot.png")
	mustWrite(t, input, pngData)
	for _, tc := range []struct {
		name, format string
		limit        int64
	}{{"auto100", "auto", 100000}, {"auto200", "auto", 200000}, {"jpeg100", "jpg", 100000}, {"explicitPNG100", "png", 100000}} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Reporter{}
			job := Job{Format: tc.format, Profile: "balanced", Limit: tc.limit}
			if err := processImage(context.Background(), job, input, t.TempDir(), r); err != nil {
				t.Fatal(err)
			}
			if len(r.Records) != 1 || r.Records[0].Status != "saved" {
				t.Fatalf("records: %+v", r.Records)
			}
			rec := r.Records[0]
			b, err := os.ReadFile(rec.Output)
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(b)) >= tc.limit || int64(len(b)) != rec.After {
				t.Fatalf("not strictly below %d: %d", tc.limit, len(b))
			}
			_, format, err := image.DecodeConfig(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			if tc.format == "png" && format != "png" {
				t.Fatal("explicit format was changed")
			}
			if tc.format == "auto" && (format != "jpeg" || !strings.Contains(rec.Message, "转为 JPG")) {
				t.Fatal("auto fallback missing or undisclosed")
			}
			if !strings.Contains(rec.Message, "检查文字") {
				t.Fatal("clarity warning missing")
			}
		})
	}
	original, err := os.ReadFile(input)
	if err != nil || !bytes.Equal(original, pngData) {
		t.Fatal("input changed")
	}
}

func TestSmallLimitKeepsClearProfileAndCancellation(t *testing.T) {
	src := sample(900, 1500, true)
	_, err := fitImage(context.Background(), src, options(Job{Profile: "clear"}, 95000, "jpg"))
	if !errors.Is(err, errCannotFit) {
		t.Fatalf("clear profile unexpectedly relaxed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = fitImage(ctx, src, options(Job{}, 95000, "jpg")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel hidden: %v", err)
	}
}

func TestSmallPDFLimitsKeepAllPages(t *testing.T) {
	src := sample(900, 1500, true)
	jpg, err := encodeImage(src, "jpg", 92)
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := makePDF([]PDFImage{{JPEG: jpg, Width: 900, Height: 1500}, {JPEG: jpg, Width: 900, Height: 1500}}, "a4")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "two-pages.pdf")
	mustWrite(t, input, pdf)
	for _, limit := range []int64{100000, 200000} {
		job := Job{Limit: limit, Profile: "balanced", DPI: 200, Format: "jpg", Pages: "1"}
		r := &Reporter{}
		if err := processPDFCompressWithRenderer(context.Background(), job, input, t.TempDir(), "", r, mockPDFRenderer(t, []*image.RGBA{src, src})); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(r.Records[0].Output)
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(b)) >= limit || bytes.Count(b, []byte("/Type /Page ")) != 2 {
			t.Fatalf("PDF limit or page preservation failed: %d", len(b))
		}
		if !strings.Contains(r.Records[0].Message, "检查文字") {
			t.Fatal("PDF clarity warning missing")
		}
	}
	job := Job{Limit: 100000, Profile: "balanced", DPI: 200, Format: "jpg", Crop: CropOptions{Mode: "none"}}
	r := &Reporter{}
	if err := processPDFImagesWithRenderer(context.Background(), job, input, t.TempDir(), "", r, mockPDFRenderer(t, []*image.RGBA{src, src})); err != nil {
		t.Fatal(err)
	}
	if len(r.Records) != 2 {
		t.Fatal("page export missing")
	}
	for _, rec := range r.Records {
		if rec.Status != "saved" || rec.After >= 100000 {
			t.Fatalf("PDF image: %+v", rec)
		}
	}
	imgPath := filepath.Join(t.TempDir(), "scan.jpg")
	mustWrite(t, imgPath, jpg)
	job.Inputs = []string{imgPath, imgPath}
	job.Paper = "a4"
	r = &Reporter{}
	if err := processImagesPDF(context.Background(), job, t.TempDir(), r); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(r.Records[0].Output)
	if err != nil || int64(len(b)) >= 100000 || bytes.Count(b, []byte("/Type /Page ")) != 2 {
		t.Fatal("merged PDF size or page count")
	}
}
