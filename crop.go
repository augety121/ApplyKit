package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"
	"os"
	"strconv"
)

// NormalizedRect uses the rendered page's top-left origin. Normalization makes
// a manual selection independent of preview DPI, display scaling and PDF units.
// The rectangle describes the region to KEEP, not the area to remove.
type NormalizedRect struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

type ManualCropPlan struct {
	SHA256 string                    `json:"sha256"`
	All    *NormalizedRect           `json:"all,omitempty"`
	Pages  map[string]NormalizedRect `json:"pages,omitempty"`
}

type CropOptions struct {
	Mode      string                    `json:"mode"` // none, auto, uniform, manual
	Threshold int                       `json:"threshold"`
	MarginMM  float64                   `json:"marginMM"`
	Manual    map[string]ManualCropPlan `json:"manual,omitempty"`
}

type CropInfo struct {
	Mode         string `json:"mode"`
	Applied      bool   `json:"applied"`
	Blank        bool   `json:"blank,omitempty"`
	SourceWidth  int    `json:"sourceWidth"`
	SourceHeight int    `json:"sourceHeight"`
	Left         int    `json:"left"`
	Top          int    `json:"top"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Note         string `json:"note"`
}

type detectedCrop struct {
	Bounds image.Rectangle
	Blank  bool
	Note   string
}

func fullRect() NormalizedRect { return NormalizedRect{0, 0, 1, 1} }
func (r NormalizedRect) validate() error {
	for _, v := range []float64{r.Left, r.Top, r.Right, r.Bottom} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return fmt.Errorf("裁剪坐标必须是 0～1 的有限数值")
		}
	}
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return fmt.Errorf("裁剪区域的宽、高必须大于零")
	}
	return nil
}
func (r NormalizedRect) pixels(b image.Rectangle) (image.Rectangle, error) {
	if err := r.validate(); err != nil {
		return image.Rectangle{}, err
	}
	// Round outwards, not inwards: a pixel touching the selection is retained.
	p := image.Rect(b.Min.X+int(math.Floor(r.Left*float64(b.Dx()))), b.Min.Y+int(math.Floor(r.Top*float64(b.Dy()))), b.Min.X+int(math.Ceil(r.Right*float64(b.Dx()))), b.Min.Y+int(math.Ceil(r.Bottom*float64(b.Dy())))).Intersect(b)
	if p.Dx() < 2 || p.Dy() < 2 {
		return image.Rectangle{}, fmt.Errorf("裁剪区域过小，请至少保留 2×2 像素")
	}
	return p, nil
}
func normalizeRect(p, b image.Rectangle) NormalizedRect {
	return NormalizedRect{float64(p.Min.X-b.Min.X) / float64(b.Dx()), float64(p.Min.Y-b.Min.Y) / float64(b.Dy()), float64(p.Max.X-b.Min.X) / float64(b.Dx()), float64(p.Max.Y-b.Min.Y) / float64(b.Dy())}
}
func unionRect(a, b NormalizedRect) NormalizedRect {
	return NormalizedRect{math.Min(a.Left, b.Left), math.Min(a.Top, b.Top), math.Max(a.Right, b.Right), math.Max(a.Bottom, b.Bottom)}
}
func (c CropOptions) mode() string {
	if c.Mode == "" {
		return "none"
	}
	return c.Mode
}
func (c CropOptions) threshold() int {
	if c.Threshold == 0 {
		return 250
	}
	return c.Threshold
}
func (c CropOptions) validate() error {
	switch c.mode() {
	case "none", "auto", "uniform", "manual":
	default:
		return fmt.Errorf("未知裁剪方式：%s", c.Mode)
	}
	if c.threshold() < 200 || c.threshold() > 254 {
		return fmt.Errorf("白边识别阈值应在 200～254 之间")
	}
	if math.IsNaN(c.MarginMM) || math.IsInf(c.MarginMM, 0) || c.MarginMM < 0 || c.MarginMM > 20 {
		return fmt.Errorf("安全边距应在 0～20 毫米之间")
	}
	if len(c.Manual) > 200 {
		return fmt.Errorf("手动裁剪方案过多")
	}
	for _, p := range c.Manual {
		if p.All != nil {
			if e := p.All.validate(); e != nil {
				return e
			}
		}
		if len(p.Pages) > 200 {
			return fmt.Errorf("单个 PDF 的手动裁剪页数超过 200")
		}
		for key, r := range p.Pages {
			n, e := strconv.Atoi(key)
			if e != nil || n < 1 || n > 200 {
				return fmt.Errorf("手动裁剪页码无效：%s", key)
			}
			if e = r.validate(); e != nil {
				return e
			}
		}
	}
	return nil
}

func marginPixels(c CropOptions, ev renderEvent, b image.Rectangle, dpi int) (int, int) {
	// PageWidth/Height are points. Use effective DPI even if renderer limits
	// reduced an unusually large page. The same physical margin is kept per axis.
	dx, dy := float64(dpi), float64(dpi)
	if dx <= 0 {
		dx = 200
		dy = 200
	}
	if ev.PageWidth > 0 {
		dx = float64(b.Dx()) * 72 / ev.PageWidth
	}
	if ev.PageHeight > 0 {
		dy = float64(b.Dy()) * 72 / ev.PageHeight
	}
	return int(math.Ceil(c.MarginMM * dx / 25.4)), int(math.Ceil(c.MarginMM * dy / 25.4))
}

// detectWhiteMargins examines EVERY pixel. It intentionally does not use row
// majority voting, OCR or noise deletion: a tiny colored stamp / footer / dot
// must not be discarded just because it occupies a small fraction of a row.
// Scans should already have been composited onto white by decodeImage.
func detectWhiteMargins(ctx context.Context, im *image.RGBA, threshold, mx, my int) (detectedCrop, error) {
	b := im.Bounds()
	res := detectedCrop{Bounds: b}
	if b.Empty() {
		return res, fmt.Errorf("不能裁剪空图像")
	}
	if threshold < 200 || threshold > 254 || mx < 0 || my < 0 {
		return res, fmt.Errorf("裁剪参数无效")
	}
	left, top, right, bottom := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if (y-b.Min.Y)%32 == 0 {
			if e := ctx.Err(); e != nil {
				return res, e
			}
		}
		off := im.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x++ {
			// any darker channel counts; averaging would lose pale red/blue markings.
			if int(im.Pix[off]) <= threshold || int(im.Pix[off+1]) <= threshold || int(im.Pix[off+2]) <= threshold {
				found = true
				left = min(left, x)
				right = max(right, x+1)
				top = min(top, y)
				bottom = max(bottom, y+1)
			}
			off += 4
		}
	}
	if !found {
		res.Blank = true
		res.Note = "未检测到超过阈值的内容；保留整页，不删除空白页"
		return res, nil
	}
	content := image.Rect(left, top, right, bottom)
	// Very sparse detections may be scanner speckles or extremely small marks.
	// Keep the full page and ask the user to inspect it rather than crop hard.
	if content.Dx() < 8 || content.Dy() < 8 || int64(content.Dx())*int64(content.Dy())*1000 < int64(b.Dx())*int64(b.Dy()) {
		res.Note = "检测内容过少，已保守地保留整页；可使用手动框选"
		return res, nil
	}
	res.Bounds = image.Rect(left-mx, top-my, right+mx, bottom+my).Intersect(b)
	if res.Bounds == b {
		res.Note = "内容接近页边，未发现可安全去除的白边"
	} else {
		res.Note = "自动去除四周白边，已保留安全边距；请核对淡色内容"
	}
	return res, nil
}
func cropRGBA(src *image.RGBA, p image.Rectangle) (*image.RGBA, error) {
	if p.Empty() || !p.In(src.Bounds()) {
		return nil, fmt.Errorf("裁剪区域超出页面")
	}
	if p == src.Bounds() && p.Min == (image.Point{}) {
		return src, nil
	}
	out := image.NewRGBA(image.Rect(0, 0, p.Dx(), p.Dy()))
	draw.Draw(out, out.Bounds(), src, p.Min, draw.Src)
	return out, nil
}
func applyPageCrop(ctx context.Context, src *image.RGBA, ev renderEvent, job Job, plan *ManualCropPlan, uniform *NormalizedRect) (*image.RGBA, *CropInfo, error) {
	mode := job.Crop.mode()
	if mode == "none" {
		return src, nil, nil
	}
	b := src.Bounds()
	p := b
	note := ""
	blank := false
	switch mode {
	case "auto", "uniform":
		mx, my := marginPixels(job.Crop, ev, b, job.DPI)
		det, e := detectWhiteMargins(ctx, src, job.Crop.threshold(), mx, my)
		if e != nil {
			return nil, nil, e
		}
		p, blank, note = det.Bounds, det.Blank, det.Note
		if mode == "uniform" && !blank && uniform != nil {
			p, e = uniform.pixels(b)
			if e != nil {
				return nil, nil, e
			}
			note = "使用该 PDF 所选页的内容并集；不同页面比例可能得到不同像素尺寸"
		}
	case "manual":
		if plan == nil {
			return nil, nil, fmt.Errorf("缺少该 PDF 的手动裁剪方案")
		}
		chosen := plan.All
		if r, ok := plan.Pages[strconv.Itoa(ev.Page)]; ok {
			chosen = &r
		}
		if chosen != nil {
			var e error
			p, e = chosen.pixels(b)
			if e != nil {
				return nil, nil, e
			}
			note = "按预览中确认的手动框选区域裁剪"
		} else {
			note = "此页没有设置手动框选，保留完整页面"
		}
	default:
		return nil, nil, fmt.Errorf("未知裁剪方式")
	}
	out, e := cropRGBA(src, p)
	if e != nil {
		return nil, nil, e
	}
	info := &CropInfo{Mode: mode, Applied: p != b, Blank: blank, SourceWidth: b.Dx(), SourceHeight: b.Dy(), Left: p.Min.X - b.Min.X, Top: p.Min.Y - b.Min.Y, Width: p.Dx(), Height: p.Dy(), Note: note}
	return out, info, nil
}
func cropNote(info *CropInfo) string {
	if info == nil {
		return ""
	}
	if !info.Applied {
		return "；裁剪：" + info.Note
	}
	return fmt.Sprintf("；裁剪 %d×%d → %d×%d（仅表示像素范围，不是体积降幅）；%s", info.SourceWidth, info.SourceHeight, info.Width, info.Height, info.Note)
}
func fileSHA256(ctx context.Context, path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() || st.Size() > maxInputBytes {
		return "", fmt.Errorf("文件无效或超过 250MB 上限")
	}
	h := sha256.New()
	buf := make([]byte, 128*1024)
	for {
		if e = ctx.Err(); e != nil {
			return "", e
		}
		n, er := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			return "", er
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
