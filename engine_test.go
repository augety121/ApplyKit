package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sample(w, h int, noise bool) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(12345))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := im.PixOffset(x, y)
			if noise {
				im.Pix[p] = uint8(rng.Intn(256))
				im.Pix[p+1] = uint8(rng.Intn(256))
				im.Pix[p+2] = uint8(rng.Intn(256))
			} else {
				im.Pix[p] = uint8(x * 255 / max(w-1, 1))
				im.Pix[p+1] = uint8(y * 255 / max(h-1, 1))
				im.Pix[p+2] = 120
			}
			im.Pix[p+3] = 255
		}
	}
	return im
}
func mustWrite(t *testing.T, path string, b []byte) {
	t.Helper()
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestParsePages(t *testing.T) {
	for _, tc := range []struct {
		s    string
		n    int
		want []int
		bad  bool
	}{
		{"", 4, []int{1, 2, 3, 4}, false}, {"3,1,2-4", 4, []int{3, 1, 2, 4}, false}, {"1，3-4", 4, []int{1, 3, 4}, false},
		{"0", 4, nil, true}, {"5", 4, nil, true}, {"3-2", 4, nil, true}, {"1,", 4, nil, true}, {"a", 4, nil, true}, {"", 201, nil, true}, {"1-3-4", 4, nil, true},
	} {
		got, e := parsePages(tc.s, tc.n)
		if (e != nil) != tc.bad {
			t.Fatalf("%q error=%v", tc.s, e)
		}
		if !tc.bad && !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q=%v", tc.s, got)
		}
	}
}
func TestJPEGUnderTwoMillion(t *testing.T) {
	im := sample(2200, 3000, true)
	f, e := fitImage(context.Background(), im, FitOptions{Limit: 1900000, Format: "jpg", MinQuality: 65, MaxQuality: 92, MinLong: 1400})
	if e != nil {
		t.Fatal(e)
	}
	if len(f.Data) > 1900000 || len(f.Data) >= 2000000 {
		t.Fatal(len(f.Data))
	}
	cfg, kind, e := image.DecodeConfig(bytes.NewReader(f.Data))
	if e != nil || kind != "jpeg" || cfg.Width != f.Width || cfg.Height != f.Height {
		t.Fatalf("bad image: %v", e)
	}
	if f.Width > 2200 || f.Height > 3000 || max(f.Width, f.Height) < 1400 {
		t.Fatal("resolution guard failed")
	}
	t.Logf("2200x3000 noise -> %d bytes, %dx%d, q=%d", len(f.Data), f.Width, f.Height, f.Quality)
}
func TestPNGAndGIFUnderCap(t *testing.T) {
	im := sample(640, 900, false)
	for _, format := range []string{"png", "gif", "jpeg"} {
		f, e := fitImage(context.Background(), im, FitOptions{Limit: 95000, Format: format, MinQuality: 65, MaxQuality: 92, MinLong: 450})
		if e != nil {
			t.Fatalf("%s: %v", format, e)
		}
		if len(f.Data) > 95000 {
			t.Fatal("over cap")
		}
		_, kind, e := image.DecodeConfig(bytes.NewReader(f.Data))
		if e != nil || normalizeFormat(kind) != normalizeFormat(format) {
			t.Fatalf("%s magic mismatch %s %v", format, kind, e)
		}
	}
}
func TestImpossibleCapStops(t *testing.T) {
	_, e := fitImage(context.Background(), sample(400, 600, false), FitOptions{Limit: 80, Format: "jpg", MinQuality: 65, MaxQuality: 92, MinLong: 600})
	if !errors.Is(e, errCannotFit) {
		t.Fatal(e)
	}
}
func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := fitImage(ctx, sample(100, 150, false), FitOptions{Limit: 50000, Format: "jpg"})
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestTransparencyWhite(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	im.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 0})
	im.SetNRGBA(1, 0, color.NRGBA{255, 0, 0, 128})
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	s, e := decodeImage(b.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	if s.Image.RGBAAt(0, 0) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("transparent background is not white")
	}
	p := s.Image.RGBAAt(1, 0)
	if p.R != 255 || p.G < 126 || p.G > 128 || p.B < 126 || p.B > 128 {
		t.Fatal(p)
	}
}
func TestOrientations(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 2, 3))
	im.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	im.SetRGBA(1, 2, color.RGBA{0, 255, 0, 255})
	for o := 2; o <= 8; o++ {
		got := orient(im, o)
		if o >= 5 && (got.Bounds().Dx() != 3 || got.Bounds().Dy() != 2) {
			t.Fatal(o)
		}
		if o < 5 && (got.Bounds().Dx() != 2 || got.Bounds().Dy() != 3) {
			t.Fatal(o)
		}
	}
	got := orient(im, 6)
	if got.RGBAAt(2, 0).R != 255 || got.RGBAAt(0, 1).G != 255 {
		t.Fatal("rotation 6 is wrong")
	}
}
func TestEXIFRotationLoad(t *testing.T) {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, sample(40, 60, false), &jpeg.Options{Quality: 90})
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	marker := []byte{255, 225, 0, 0}
	binary.BigEndian.PutUint16(marker[2:], uint16(len(payload)+2))
	data := append([]byte{}, b.Bytes()[:2]...)
	data = append(data, marker...)
	data = append(data, payload...)
	data = append(data, b.Bytes()[2:]...)
	if exifOrientation(data) != 6 {
		t.Fatal("exif parser failed")
	}
	src, e := decodeImage(data)
	if e != nil {
		t.Fatal(e)
	}
	if !src.Oriented || src.Image.Bounds().Dx() != 60 || src.Image.Bounds().Dy() != 40 {
		t.Fatal("EXIF was not applied")
	}
}
func TestAnimatedGIFRejected(t *testing.T) {
	p := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.White, color.Black})
	a := gif.GIF{Image: []*image.Paletted{p, p}, Delay: []int{5, 5}}
	var b bytes.Buffer
	if e := gif.EncodeAll(&b, &a); e != nil {
		t.Fatal(e)
	}
	if gifFrameCount(b.Bytes()) != 2 {
		t.Fatal("frames miscounted")
	}
	src, e := decodeImage(b.Bytes())
	if e != nil || !src.Animated {
		t.Fatalf("animation not rejected %v", e)
	}
}
func TestAtomicUniqueAndOriginal(t *testing.T) {
	dir := t.TempDir()
	p1, e := saveUnique(dir, "材料.jpg", []byte("first"))
	if e != nil {
		t.Fatal(e)
	}
	p2, e := saveUnique(dir, "材料.jpg", []byte("second"))
	if e != nil {
		t.Fatal(e)
	}
	if p1 == p2 {
		t.Fatal("overwrite")
	}
	b, _ := os.ReadFile(p1)
	if string(b) != "first" {
		t.Fatal("original changed")
	}
	if _, e = saveUnique(dir, "../escape.jpg", []byte("bad")); e == nil {
		t.Fatal("path traversal allowed")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 2 {
		t.Fatal("temp leaked")
	}
}
func TestPDFStructure(t *testing.T) {
	j, _ := encodeImage(sample(160, 240, false), "jpg", 85)
	b, e := makePDF([]PDFImage{{JPEG: j, Width: 160, Height: 240}, {JPEG: j, Width: 160, Height: 240, PageWidth: 612, PageHeight: 792}}, "a4")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-1.4")) || !bytes.Contains(b, []byte("/Count 2")) || !bytes.HasSuffix(b, []byte("%%EOF\n")) {
		t.Fatal("bad PDF structure")
	}
	if _, e = makePDF(nil, "a4"); e == nil {
		t.Fatal("empty PDF accepted")
	}
}
func TestSafeName(t *testing.T) {
	if safeName("CON.pdf") == "CON" {
		t.Fatal("reserved name not escaped")
	}
	s := safeName("a:b?.pdf")
	if strings.ContainsAny(s, ":?") {
		t.Fatal(s)
	}
}
func TestCompressionJobShrinksAndKeepsInputs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "测试材料.png")
	var b bytes.Buffer
	_ = png.Encode(&b, sample(1600, 2000, true))
	original := append([]byte{}, b.Bytes()...)
	mustWrite(t, src, original)
	progress := filepath.Join(dir, "progress.jsonl")
	job := Job{Mode: "image-compress", Inputs: []string{src}, Output: filepath.Join(dir, "out"), Format: "jpg", Limit: 2000000, Profile: "balanced", Progress: progress, MakeZip: true}
	if code := runJob(job, dir); code != 0 {
		t.Fatal(code)
	}
	after, _ := os.ReadFile(src)
	if !bytes.Equal(after, original) {
		t.Fatal("input modified")
	}
	events, _ := os.ReadFile(progress)
	saved := false
	for _, line := range bytes.Split(events, []byte("\n")) {
		var ev Event
		if json.Unmarshal(line, &ev) == nil && ev.Record != nil && ev.Record.Status == "saved" {
			saved = true
			rec := ev.Record
			if rec.After >= rec.Before || rec.After >= 2000000 {
				t.Fatal("size check failed")
			}
			out, _ := os.ReadFile(rec.Output)
			if int64(len(out)) != rec.After {
				t.Fatal("report size wrong")
			}
		}
	}
	if !saved {
		t.Fatalf("no saved event: %s", events)
	}
}
func TestAlreadyTinyOriginalNotMisreported(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "tiny.jpg")
	var b bytes.Buffer
	_ = jpeg.Encode(&b, sample(64, 96, true), &jpeg.Options{Quality: 3})
	mustWrite(t, src, b.Bytes())
	progress := filepath.Join(dir, "progress.jsonl")
	job := Job{Mode: "image-compress", Inputs: []string{src}, Output: filepath.Join(dir, "out"), Format: "jpg", Limit: 2000000, Progress: progress}
	if runJob(job, dir) != 0 {
		t.Fatal("job failed")
	}
	events, _ := os.ReadFile(progress)
	kept := false
	for _, line := range bytes.Split(events, []byte("\n")) {
		var ev Event
		if json.Unmarshal(line, &ev) == nil && ev.Record != nil {
			if ev.Record.Status != "unchanged" {
				t.Fatalf("expected unchanged, got %+v", ev.Record)
			}
			kept = true
			out, _ := os.ReadFile(ev.Record.Output)
			if !bytes.Equal(out, b.Bytes()) {
				t.Fatal("retained bytes changed")
			}
		}
	}
	if !kept {
		t.Fatal("no record")
	}
}
func TestMergeJobAndStrictFailure(t *testing.T) {
	for _, strict := range []bool{false, true} {
		dir := t.TempDir()
		src := filepath.Join(dir, "plain.png")
		var b bytes.Buffer
		_ = png.Encode(&b, sample(100, 140, false))
		mustWrite(t, src, b.Bytes())
		progress := filepath.Join(dir, "progress.jsonl")
		job := Job{Mode: "images-pdf", Inputs: []string{src, src}, Output: filepath.Join(dir, "out"), Format: "jpg", Limit: 2000000, Progress: progress, Paper: "a4", Strict: strict}
		if runJob(job, dir) != 0 {
			t.Fatal("job failed")
		}
		events, _ := os.ReadFile(progress)
		found := false
		for _, line := range bytes.Split(events, []byte("\n")) {
			var ev Event
			if json.Unmarshal(line, &ev) == nil && ev.Record != nil {
				found = true
				if strict && ev.Record.Status != "failed" {
					t.Fatal("strict conversion incorrectly succeeded")
				}
				if !strict {
					if ev.Record.Status != "saved" {
						t.Fatalf("merge failed %+v", ev.Record)
					}
					out, _ := os.ReadFile(ev.Record.Output)
					if !bytes.Contains(out, []byte("/Count 2")) {
						t.Fatal("lost pages")
					}
				}
			}
		}
		if !found {
			t.Fatal("no record")
		}
	}
}
