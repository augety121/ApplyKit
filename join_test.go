package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func solidJoin(w, h int, c color.Color) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(im, im.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return im
}
func joinFixture(t *testing.T, im *image.RGBA) string {
	t.Helper()
	b, e := encodeImage(im, "png", 92)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "part.png")
	mustWrite(t, p, b)
	return p
}

func TestJoinImagesOrderDirectionAndEdits(t *testing.T) {
	red := joinFixture(t, solidJoin(80, 40, color.RGBA{255, 0, 0, 255}))
	blue := joinFixture(t, solidJoin(40, 80, color.RGBA{0, 0, 255, 255}))
	editIdentity, err := fileSHA256(context.Background(), blue)
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{"vertical", "horizontal"} {
		t.Run(direction, func(t *testing.T) {
			job := Job{JoinImage: true, JoinDirection: direction, Mode: "image-compress", Inputs: []string{blue, red}, Format: "png", Limit: 100000, Edits: map[string]ImageEdit{blue: {Rotation: 90, SHA256: editIdentity}}}
			r := &Reporter{}
			out := t.TempDir()
			if e := processJoinedImage(context.Background(), job, out, "", r, nil); e != nil {
				t.Fatal(e)
			}
			if len(r.Records) != 1 {
				t.Fatalf("want one output: %+v", r.Records)
			}
			b, e := os.ReadFile(r.Records[0].Output)
			if e != nil {
				t.Fatal(e)
			}
			im, format, e := image.Decode(bytes.NewReader(b))
			if e != nil || format != "png" {
				t.Fatal(e, format)
			}
			w, h := 80, 80
			if direction == "horizontal" {
				w, h = 160, 40
			}
			if im.Bounds().Dx() != w || im.Bounds().Dy() != h {
				t.Fatalf("edit/aspect ratio lost: %v", im.Bounds())
			}
			first := im.At(20, 20)
			x, y := 20, 60
			if direction == "horizontal" {
				x, y = 100, 20
			}
			last := im.At(x, y)
			fr, _, fb, _ := first.RGBA()
			lr, _, lb, _ := last.RGBA()
			if fb <= fr || lr <= lb {
				t.Fatal("queue order not preserved")
			}
			entries, _ := os.ReadDir(out)
			if len(entries) != 1 {
				t.Fatalf("intermediate files leaked: %v", entries)
			}
		})
	}
}

func TestJoinPDFPagesAndSmallLimits(t *testing.T) {
	a := sample(600, 900, true)
	b := solidJoin(600, 900, color.White)
	input := filepath.Join(t.TempDir(), "pages.pdf")
	mustWrite(t, input, []byte("synthetic renderer input"))
	for _, limit := range []int64{100000, 200000, 300000} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			job := Job{JoinImage: true, Mode: "pdf-images", Inputs: []string{input}, Format: "jpg", Limit: limit, Crop: CropOptions{Mode: "auto"}}
			r := &Reporter{}
			if e := processJoinedImage(context.Background(), job, t.TempDir(), "", r, mockPDFRenderer(t, []*image.RGBA{a, b})); e != nil {
				t.Fatal(e)
			}
			if len(r.Records) != 1 || r.Records[0].After >= limit || !strings.Contains(r.Records[0].Message, "共 2 页/张") {
				t.Fatalf("missing pages or limit: %+v", r.Records)
			}
			data, _ := os.ReadFile(r.Records[0].Output)
			im, _, e := image.Decode(bytes.NewReader(data))
			if e != nil {
				t.Fatal(e)
			}
			red, green, blue, _ := im.At(im.Bounds().Dx()/2, im.Bounds().Dy()*7/8).RGBA()
			if red < 60000 || green < 60000 || blue < 60000 {
				t.Fatal("blank second page disappeared")
			}
		})
	}
}

func TestJoinFailureNeverPublishesPartialImage(t *testing.T) {
	p := joinFixture(t, solidJoin(50, 50, color.White))
	out := t.TempDir()
	r := &Reporter{}
	job := Job{JoinImage: true, Mode: "image-compress", Inputs: []string{p, p + "missing"}, Format: "jpg", Limit: 100000}
	if e := processJoinedImage(context.Background(), job, out, "", r, nil); e == nil {
		t.Fatal("missing input accepted")
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 0 || len(r.Records) != 0 {
		t.Fatal("partial output leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	job.Inputs = []string{p}
	if e := processJoinedImage(ctx, job, out, "", r, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	job.JoinDirection = "diagonal"
	if validateJoin(job) == nil {
		t.Fatal("invalid direction accepted")
	}
	job.JoinDirection = ""
	job.Mode = "images-pdf"
	if validateJoin(job) == nil {
		t.Fatal("invalid join mode accepted")
	}
}

func TestJoinLayoutBoundedCanvas(t *testing.T) {
	parts := make([]joinPart, 100)
	for i := range parts {
		parts[i] = joinPart{w: 1, h: 2400}
	}
	for _, direction := range []string{"vertical", "horizontal"} {
		rects, w, h := joinLayout(parts, direction, 0)
		if w > 12000 || h > 12000 || int64(w)*int64(h) > maxPixels {
			t.Fatalf("unbounded %dx%d", w, h)
		}
		for _, r := range rects {
			if r.Empty() {
				t.Fatal("source disappeared")
			}
		}
	}
}
