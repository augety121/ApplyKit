package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func whitePage(w, h int, content image.Rectangle) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(im, im.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	if !content.Empty() {
		draw.Draw(im, content, image.NewUniform(color.RGBA{25, 45, 70, 255}), image.Point{}, draw.Src)
	}
	return im
}
func TestAutoCropWhiteMargins(t *testing.T) {
	im := whitePage(1000, 1400, image.Rect(200, 250, 800, 1150))
	got, e := detectWhiteMargins(context.Background(), im, 250, 16, 16)
	if e != nil || got.Bounds != image.Rect(184, 234, 816, 1166) || got.Blank {
		t.Fatalf("%+v %v", got, e)
	}
}
func TestAutoCropBlankPageRetained(t *testing.T) {
	im := whitePage(300, 400, image.Rectangle{})
	got, e := detectWhiteMargins(context.Background(), im, 250, 10, 10)
	if e != nil || !got.Blank || got.Bounds != im.Bounds() {
		t.Fatalf("%+v %v", got, e)
	}
}
func TestAutoCropProtectsSingleColoredEdgePixel(t *testing.T) {
	im := whitePage(600, 800, image.Rect(100, 100, 500, 650))
	im.SetRGBA(3, 797, color.RGBA{255, 240, 250, 255})
	got, e := detectWhiteMargins(context.Background(), im, 250, 0, 0)
	if e != nil || got.Bounds.Min.X > 3 || got.Bounds.Max.Y <= 797 {
		t.Fatalf("edge mark lost: %+v %v", got, e)
	}
}
func TestPureWhiteModeKeepsFaintMark(t *testing.T) {
	im := whitePage(600, 800, image.Rect(100, 100, 500, 650))
	im.SetRGBA(12, 775, color.RGBA{253, 253, 253, 255})
	got, e := detectWhiteMargins(context.Background(), im, 254, 0, 0)
	if e != nil || got.Bounds.Min.X > 12 || got.Bounds.Max.Y <= 775 {
		t.Fatal(got, e)
	}
}
func TestAutoThresholdHasExplicitTradeoff(t *testing.T) {
	im := whitePage(600, 800, image.Rect(100, 100, 500, 650))
	im.SetRGBA(12, 775, color.RGBA{253, 253, 253, 255})
	got, e := detectWhiteMargins(context.Background(), im, 250, 0, 0)
	if e != nil || got.Bounds != image.Rect(100, 100, 500, 650) {
		t.Fatal(got, e)
	}
	// This explicitly documents the reason for the preview warning: no automatic
	// threshold can promise to retain every nearly-white document mark.
}
func TestAutoCropSparseContentRetained(t *testing.T) {
	im := whitePage(1000, 1400, image.Rect(500, 600, 505, 605))
	got, e := detectWhiteMargins(context.Background(), im, 250, 0, 0)
	if e != nil || got.Bounds != im.Bounds() || got.Blank {
		t.Fatal(got, e)
	}
}
func TestAutoCropFullBleedRetained(t *testing.T) {
	im := whitePage(400, 600, image.Rect(0, 0, 400, 600))
	got, e := detectWhiteMargins(context.Background(), im, 250, 10, 10)
	if e != nil || got.Bounds != im.Bounds() {
		t.Fatal(got, e)
	}
}
func TestCropCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := detectWhiteMargins(ctx, whitePage(400, 600, image.Rect(50, 50, 350, 550)), 250, 10, 10)
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestCropNonzeroOrigin(t *testing.T) {
	im := image.NewRGBA(image.Rect(40, 70, 440, 670))
	draw.Draw(im, im.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	content := image.Rect(100, 130, 340, 570)
	draw.Draw(im, content, image.NewUniform(color.Black), image.Point{}, draw.Src)
	got, e := detectWhiteMargins(context.Background(), im, 250, 0, 0)
	if e != nil || got.Bounds != content {
		t.Fatal(got, e)
	}
	cropped, e := cropRGBA(im, got.Bounds)
	if e != nil || cropped.Bounds() != image.Rect(0, 0, 240, 440) || cropped.RGBAAt(0, 0) != (color.RGBA{0, 0, 0, 255}) {
		t.Fatal(e)
	}
}
func TestNormalizedRectRejectsInvalidValues(t *testing.T) {
	cases := []NormalizedRect{{-.1, 0, 1, 1}, {0, 0, 1.1, 1}, {0, 0, 0, 1}, {0, .6, 1, .5}, {0, math.NaN(), 1, 1}, {0, 0, math.Inf(1), 1}}
	for _, r := range cases {
		if r.validate() == nil {
			t.Fatal(r)
		}
	}
	if _, e := (NormalizedRect{0, 0, .00001, 1}).pixels(image.Rect(0, 0, 800, 1000)); e == nil {
		t.Fatal("subpixel frame accepted")
	}
}
func TestNormalizedRectRoundsOutwards(t *testing.T) {
	r := NormalizedRect{.1001, .2001, .8999, .8001}
	p, e := r.pixels(image.Rect(0, 0, 1000, 1000))
	if e != nil || p != image.Rect(100, 200, 900, 801) {
		t.Fatal(p, e)
	}
}
func TestNormalizedRectDPIIndependent(t *testing.T) {
	r := NormalizedRect{.1, .2, .9, .8}
	a, _ := r.pixels(image.Rect(0, 0, 1000, 1500))
	b, _ := r.pixels(image.Rect(0, 0, 2000, 3000))
	if b.Min.X != 2*a.Min.X || b.Min.Y != 2*a.Min.Y || b.Dx() != 2*a.Dx() || b.Dy() != 2*a.Dy() {
		t.Fatal(a, b)
	}
}
func TestSafetyMarginUsesEffectiveDPI(t *testing.T) {
	c := CropOptions{MarginMM: 2.54}
	ev := renderEvent{PageWidth: 720, PageHeight: 1080}
	mx, my := marginPixels(c, ev, image.Rect(0, 0, 1000, 1500), 300)
	if mx != 10 || my != 10 {
		t.Fatalf("margin used requested rather than effective DPI: %d %d", mx, my)
	}
}
func TestUniformUsesUnionNotIntersection(t *testing.T) {
	a := NormalizedRect{.1, .1, .7, .8}
	b := NormalizedRect{.3, .2, .9, .95}
	u := unionRect(a, b)
	if u != (NormalizedRect{.1, .1, .9, .95}) {
		t.Fatal(u)
	}
}
func TestManualPageOverridesAll(t *testing.T) {
	all := NormalizedRect{.1, .1, .9, .9}
	specific := NormalizedRect{.2, .25, .8, .75}
	plan := &ManualCropPlan{All: &all, Pages: map[string]NormalizedRect{"2": specific}}
	job := Job{DPI: 200, Crop: CropOptions{Mode: "manual"}}
	im := whitePage(1000, 1400, image.Rectangle{})
	_, info, e := applyPageCrop(context.Background(), im, renderEvent{Page: 2}, job, plan, nil)
	if e != nil || info.Left != 200 || info.Top != 350 || info.Width != 600 || info.Height != 700 {
		t.Fatal(info, e)
	}
	_, info, e = applyPageCrop(context.Background(), im, renderEvent{Page: 1}, job, plan, nil)
	if e != nil || info.Left != 100 || info.Width != 800 {
		t.Fatal(info, e)
	}
}
func TestManualUnsetPageRetained(t *testing.T) {
	im := whitePage(600, 800, image.Rectangle{})
	plan := &ManualCropPlan{Pages: map[string]NormalizedRect{"1": {.2, .2, .8, .8}}}
	out, info, e := applyPageCrop(context.Background(), im, renderEvent{Page: 2}, Job{Crop: CropOptions{Mode: "manual"}}, plan, nil)
	if e != nil || out != im || info.Applied {
		t.Fatal(info, e)
	}
}
func TestCropNoneUnchanged(t *testing.T) {
	im := whitePage(400, 600, image.Rectangle{})
	out, info, e := applyPageCrop(context.Background(), im, renderEvent{}, Job{}, nil, nil)
	if e != nil || out != im || info != nil {
		t.Fatal(info, e)
	}
}
func TestCropDoesNotMutateOriginalPixels(t *testing.T) {
	im := whitePage(400, 600, image.Rect(100, 100, 300, 500))
	original := append([]byte{}, im.Pix...)
	cropped, e := cropRGBA(im, image.Rect(100, 100, 300, 500))
	if e != nil {
		t.Fatal(e)
	}
	cropped.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	if !bytes.Equal(im.Pix, original) {
		t.Fatal("crop shares mutable pixel storage")
	}
}
func TestCropOptionsValidation(t *testing.T) {
	for _, c := range []CropOptions{{Mode: "other"}, {Threshold: 199}, {Threshold: 255}, {MarginMM: -1}, {MarginMM: 21}, {MarginMM: math.NaN()}, {Manual: map[string]ManualCropPlan{"a": {Pages: map[string]NormalizedRect{"0": fullRect()}}}}} {
		if c.validate() == nil {
			t.Fatal(c)
		}
	}
	if (CropOptions{}).validate() != nil {
		t.Fatal("default options not backwards compatible")
	}
}

func newCropReporter(t *testing.T) (*Reporter, string) {
	t.Helper()
	dir := t.TempDir()
	f, e := os.Create(filepath.Join(dir, "events.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.Close() })
	return &Reporter{f: f}, dir
}

// mockPDFRenderer supplies real PNGs but does NOT exercise Windows.Data.Pdf.
// Tests using this dependency test orchestration, page selection and cropping.
func mockPDFRenderer(t *testing.T, images []*image.RGBA) PDFRenderFunc {
	t.Helper()
	return func(ctx context.Context, job Job, input, assets string, cb func(renderEvent) error) error {
		pages, e := parsePages(job.Pages, len(images))
		if e != nil {
			return e
		}
		if e = cb(renderEvent{Kind: "meta", Selected: len(pages), Total: len(images)}); e != nil {
			return e
		}
		for _, n := range pages {
			if e = ctx.Err(); e != nil {
				return e
			}
			b, e := encodeImage(images[n-1], "png", 90)
			if e != nil {
				return e
			}
			temp, e := os.MkdirTemp(t.TempDir(), "mock-render-")
			if e != nil {
				return e
			}
			path := filepath.Join(temp, "page.png")
			mustWrite(t, path, b)
			bounds := images[n-1].Bounds()
			e = cb(renderEvent{Kind: "page", Page: n, Path: path, Width: bounds.Dx(), Height: bounds.Dy(), PageWidth: float64(bounds.Dx()) * 72 / 200, PageHeight: float64(bounds.Dy()) * 72 / 200})
			_ = os.RemoveAll(temp)
			if e != nil {
				return e
			}
		}
		return nil
	}
}
func mockPDFInput(t *testing.T, dir string, size int) string {
	t.Helper()
	p := filepath.Join(dir, "source.pdf")
	mustWrite(t, p, bytes.Repeat([]byte("x"), size))
	return p
}
func TestPDFCropExportsAllFourImageExtensions(t *testing.T) {
	for _, format := range []string{"jpg", "jpeg", "png", "gif"} {
		t.Run(format, func(t *testing.T) {
			r, dir := newCropReporter(t)
			input := mockPDFInput(t, dir, 100000)
			before, _ := os.ReadFile(input)
			im := whitePage(800, 1000, image.Rect(150, 200, 650, 800))
			job := Job{DPI: 200, Format: format, Limit: 2000000, Crop: CropOptions{Mode: "auto", Threshold: 250, MarginMM: 2}}
			e := processPDFImagesWithRenderer(context.Background(), job, input, dir, "", r, mockPDFRenderer(t, []*image.RGBA{im}))
			if e != nil || len(r.Records) != 1 || r.Success != 1 {
				t.Fatalf("%v %+v", e, r.Records)
			}
			rec := r.Records[0]
			if rec.Crop == nil || !rec.Crop.Applied || rec.Width != 532 || rec.Height != 632 || rec.After >= 2000000 {
				t.Fatal(rec)
			}
			data, _ := os.ReadFile(rec.Output)
			_, kind, e := image.DecodeConfig(bytes.NewReader(data))
			if e != nil || normalizeFormat(kind) != normalizeFormat(format) {
				t.Fatal(kind, e)
			}
			if filepath.Ext(rec.Output) != "."+format {
				t.Fatal(rec.Output)
			}
			after, _ := os.ReadFile(input)
			if !bytes.Equal(before, after) {
				t.Fatal("original PDF changed")
			}
		})
	}
}
func TestUniformPDFSelectedPagesAndBlank(t *testing.T) {
	r, dir := newCropReporter(t)
	input := mockPDFInput(t, dir, 100000)
	ims := []*image.RGBA{whitePage(1000, 1400, image.Rect(100, 200, 700, 1100)), whitePage(1000, 1400, image.Rect(300, 100, 900, 1200)), whitePage(1000, 1400, image.Rectangle{}), whitePage(1000, 1400, image.Rect(0, 0, 1000, 1400))}
	calls := 0
	base := mockPDFRenderer(t, ims)
	render := func(c context.Context, j Job, i, a string, cb func(renderEvent) error) error {
		calls++
		return base(c, j, i, a, cb)
	}
	job := Job{DPI: 200, Format: "png", Limit: 2000000, Pages: "1-3", Crop: CropOptions{Mode: "uniform", Threshold: 250}}
	e := processPDFImagesWithRenderer(context.Background(), job, input, dir, "", r, render)
	if e != nil || calls != 2 || len(r.Records) != 3 {
		t.Fatal(e, calls, len(r.Records))
	}
	for _, rec := range r.Records[:2] {
		if rec.Width != 800 || rec.Height != 1100 || rec.Crop.Left != 100 || rec.Crop.Top != 100 {
			t.Fatal(rec)
		}
	}
	if r.Records[2].Crop.Applied || !r.Records[2].Crop.Blank || r.Records[2].Width != 1000 {
		t.Fatal("blank page was lost or cropped")
	}
}
func TestManualPDFHashProtection(t *testing.T) {
	r, dir := newCropReporter(t)
	input := mockPDFInput(t, dir, 1000)
	hash, _ := fileSHA256(context.Background(), input)
	rect := NormalizedRect{.1, .2, .9, .8}
	job := Job{DPI: 200, Format: "jpg", Limit: 2000000, Crop: CropOptions{Mode: "manual", Manual: map[string]ManualCropPlan{input: {SHA256: hash, All: &rect}}}}
	mustWrite(t, input, []byte("changed"))
	e := processPDFImagesWithRenderer(context.Background(), job, input, dir, "", r, mockPDFRenderer(t, []*image.RGBA{whitePage(800, 1000, image.Rectangle{})}))
	if e == nil || len(r.Records) > 0 || !strings.Contains(e.Error(), "更改") {
		t.Fatal(e, r.Records)
	}
}
func TestManualMissingPlanFailsExplicitly(t *testing.T) {
	r, dir := newCropReporter(t)
	input := mockPDFInput(t, dir, 1000)
	e := processPDFImagesWithRenderer(context.Background(), Job{Crop: CropOptions{Mode: "manual"}}, input, dir, "", r, mockPDFRenderer(t, nil))
	if e == nil || !strings.Contains(e.Error(), "尚未") {
		t.Fatal(e)
	}
}
func TestStrictCropAggregateSmallerThanSource(t *testing.T) {
	r, dir := newCropReporter(t)
	input := mockPDFInput(t, dir, 80000)
	im := whitePage(800, 1000, image.Rect(150, 200, 650, 800))
	job := Job{DPI: 200, Format: "jpg", Limit: 2000000, Strict: true, Crop: CropOptions{Mode: "auto", MarginMM: 2}}
	e := processPDFImagesWithRenderer(context.Background(), job, input, dir, "", r, mockPDFRenderer(t, []*image.RGBA{im, im}))
	if e != nil || r.Success != 2 {
		t.Fatal(e, r.Records)
	}
	var total int64
	for _, rec := range r.Records {
		total += rec.After
	}
	if total >= 80000 {
		t.Fatal(total)
	}
}
func TestCropImpossibleBudgetNotMisreported(t *testing.T) {
	r, dir := newCropReporter(t)
	input := mockPDFInput(t, dir, 10)
	im := whitePage(800, 1000, image.Rect(150, 200, 650, 800))
	job := Job{DPI: 200, Format: "jpg", Limit: 2000000, Strict: true, Crop: CropOptions{Mode: "auto", MarginMM: 2}}
	e := processPDFImagesWithRenderer(context.Background(), job, input, dir, "", r, mockPDFRenderer(t, []*image.RGBA{im}))
	if e != nil || r.Failed != 1 || r.Success != 0 || r.Records[0].Output != "" {
		t.Fatal(e, r.Records)
	}
}
func TestCropManualExportAtDifferentResolution(t *testing.T) {
	r, dir := newCropReporter(t)
	input := mockPDFInput(t, dir, 10000)
	hash, _ := fileSHA256(context.Background(), input)
	rect := NormalizedRect{.25, .2, .75, .8}
	job := Job{DPI: 300, Format: "jpg", Limit: 2000000, Crop: CropOptions{Mode: "manual", Manual: map[string]ManualCropPlan{input: {SHA256: hash, Pages: map[string]NormalizedRect{"1": rect}}}}}
	im := whitePage(2000, 3000, image.Rect(500, 600, 1500, 2400))
	e := processPDFImagesWithRenderer(context.Background(), job, input, dir, "", r, mockPDFRenderer(t, []*image.RGBA{im}))
	if e != nil || r.Success != 1 {
		t.Fatal(e, r.Records)
	}
	p, _ := rect.pixels(im.Bounds())
	if r.Records[0].Width != p.Dx() || r.Records[0].Height != p.Dy() {
		t.Fatal(r.Records[0], p)
	}
}
func TestPreviewGeneratesManifestAndRetainsOriginal(t *testing.T) {
	dir := t.TempDir()
	input := mockPDFInput(t, dir, 10000)
	hash, _ := fileSHA256(context.Background(), input)
	ims := []*image.RGBA{whitePage(800, 1000, image.Rect(150, 200, 650, 800)), whitePage(800, 1000, image.Rectangle{})}
	var events []CropPreviewEvent
	base := mockPDFRenderer(t, ims)
	render := func(c context.Context, j Job, i, a string, cb func(renderEvent) error) error {
		if j.DPI != 110 || j.RenderMaxLong != 1600 {
			t.Fatal(j)
		}
		return base(c, j, i, a, cb)
	}
	e := prepareCropPreview(context.Background(), CropPreviewRequest{Input: input, Output: filepath.Join(dir, "preview"), MarginMM: 2}, "", func(ev CropPreviewEvent) { events = append(events, ev) }, render)
	if e != nil || len(events) != 3 {
		t.Fatal(e, len(events))
	}
	if events[0].SHA256 != hash || events[0].Total != 2 || events[1].Rect == nil || !events[2].Blank {
		t.Fatal(events)
	}
	for _, ev := range events[1:] {
		if _, e = os.Stat(ev.Path); e != nil {
			t.Fatal(e)
		}
	}
	after, _ := fileSHA256(context.Background(), input)
	if after != hash {
		t.Fatal("preview modified source")
	}
}
func TestPreviewDetectsChangesDuringRendering(t *testing.T) {
	dir := t.TempDir()
	input := mockPDFInput(t, dir, 10000)
	base := mockPDFRenderer(t, []*image.RGBA{whitePage(400, 600, image.Rectangle{})})
	render := func(c context.Context, j Job, i, a string, cb func(renderEvent) error) error {
		e := base(c, j, i, a, cb)
		mustWrite(t, input, []byte("changed while rendering"))
		return e
	}
	e := prepareCropPreview(context.Background(), CropPreviewRequest{Input: input, Output: filepath.Join(dir, "preview")}, "", func(CropPreviewEvent) {}, render)
	if e == nil || !strings.Contains(e.Error(), "已更改") {
		t.Fatal(e)
	}
}
func TestCropRejectedInOtherModes(t *testing.T) {
	dir := t.TempDir()
	job := Job{Mode: "images-pdf", Inputs: []string{"unused.png"}, Output: filepath.Join(dir, "out"), Progress: filepath.Join(dir, "events"), Limit: 2000000, Crop: CropOptions{Mode: "auto"}}
	if runJob(job, "") != 2 {
		t.Fatal("crop silently ignored in another mode")
	}
}
func TestCropReportContainsActualBounds(t *testing.T) {
	r, dir := newCropReporter(t)
	r.add(Record{Input: "document.pdf", Status: "saved", After: 1234, Crop: &CropInfo{Mode: "auto", Applied: true, SourceWidth: 1000, SourceHeight: 1400, Left: 100, Top: 200, Width: 800, Height: 1000}})
	if e := writeReport(Job{Mode: "pdf-images", Limit: 2000000}, dir, r); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(filepath.Join(dir, "处理报告.json"))
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Records []Record `json:"records"`
	}
	if e = json.Unmarshal(data, &doc); e != nil {
		t.Fatal(e)
	}
	if len(doc.Records) != 1 || doc.Records[0].Crop == nil || doc.Records[0].Crop.Left != 100 {
		t.Fatal(string(data))
	}
}
func TestCancelledUniformExportsNothing(t *testing.T) {
	r, dir := newCropReporter(t)
	input := mockPDFInput(t, dir, 10000)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := processPDFImagesWithRenderer(ctx, Job{DPI: 200, Format: "jpg", Limit: 2000000, Crop: CropOptions{Mode: "uniform"}}, input, dir, "", r, mockPDFRenderer(t, []*image.RGBA{whitePage(400, 600, image.Rectangle{})}))
	if !errors.Is(e, context.Canceled) || r.Success != 0 {
		t.Fatal(e, r.Records)
	}
}
func TestManualPlanJSONRoundtrip(t *testing.T) {
	key := `C:\Users\测试用户\投递材料\简历.pdf`
	rect := NormalizedRect{.1, .2, .9, .8}
	job := Job{Crop: CropOptions{Mode: "manual", Manual: map[string]ManualCropPlan{key: {SHA256: strings.Repeat("a", 64), All: &rect, Pages: map[string]NormalizedRect{strconv.Itoa(2): fullRect()}}}}}
	data, e := json.Marshal(job)
	if e != nil {
		t.Fatal(e)
	}
	var parsed Job
	if e = json.Unmarshal(data, &parsed); e != nil {
		t.Fatal(e)
	}
	if parsed.Crop.Manual[key].All == nil || *parsed.Crop.Manual[key].All != rect || parsed.Crop.Manual[key].Pages["2"] != fullRect() {
		t.Fatal(fmt.Sprintf("%+v", parsed))
	}
}
