package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"strings"
)

const maxInputBytes int64 = 250 * 1024 * 1024
const maxPixels int64 = 32000000

var errCannotFit = errors.New("达到清晰度底线后仍无法满足大小要求；请改用 JPG、放宽上限，或选择体积优先")

type SourceImage struct {
	Image    *image.RGBA
	Format   string
	Bytes    []byte
	Animated bool
	Oriented bool
}
type Fitted struct {
	Data    []byte
	Width   int
	Height  int
	Quality int
	Format  string
	Scaled  bool
}
type FitOptions struct {
	Limit      int64
	Format     string
	MaxQuality int
	MinQuality int
	MinLong    int
}

func readLimited(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("不是普通文件")
	}
	if st.Size() > maxInputBytes {
		return nil, fmt.Errorf("单个输入超过 250MB 安全上限")
	}
	b := make([]byte, st.Size())
	n := 0
	for n < len(b) {
		m, er := f.Read(b[n:])
		n += m
		if er != nil {
			if n == len(b) {
				break
			}
			return nil, er
		}
	}
	return b, nil
}
func loadImage(path string) (*SourceImage, error) {
	b, e := readLimited(path)
	if e != nil {
		return nil, e
	}
	return decodeImage(b)
}
func decodeImage(b []byte) (*SourceImage, error) {
	cfg, format, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil {
		return nil, fmt.Errorf("无法识别图片（支持 JPG / PNG / 静态 GIF）：%w", e)
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("图片尺寸超过 3200 万像素安全上限")
	}
	animated := format == "gif" && gifFrameCount(b) > 1
	if animated {
		return &SourceImage{Format: format, Bytes: b, Animated: true}, nil
	}
	im, _, e := image.Decode(bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	rgba := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	draw.Draw(rgba, rgba.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(rgba, rgba.Bounds(), im, im.Bounds().Min, draw.Over)
	oriented := false
	if format == "jpeg" {
		o := exifOrientation(b)
		if o > 1 && o <= 8 {
			rgba = orient(rgba, o)
			oriented = true
		}
	}
	return &SourceImage{Image: rgba, Format: normalizeFormat(format), Bytes: b, Oriented: oriented}, nil
}
func normalizeFormat(f string) string {
	f = strings.ToLower(f)
	if f == "jpeg" {
		return "jpg"
	}
	return f
}
func encodeImage(im image.Image, format string, q int) ([]byte, error) {
	var b bytes.Buffer
	var err error
	switch normalizeFormat(format) {
	case "jpg":
		err = jpeg.Encode(&b, im, &jpeg.Options{Quality: q})
	case "png":
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		err = enc.Encode(&b, im)
	case "gif":
		err = gif.Encode(&b, im, &gif.Options{NumColors: 256})
	default:
		return nil, fmt.Errorf("不支持的输出格式：%s", format)
	}
	return b.Bytes(), err
}
func fitImage(ctx context.Context, src *image.RGBA, opt FitOptions) (Fitted, error) {
	var zero Fitted
	if opt.Limit <= 0 {
		return zero, errCannotFit
	}
	f := normalizeFormat(opt.Format)
	if f != "jpg" && f != "png" && f != "gif" {
		return zero, fmt.Errorf("不支持的输出格式")
	}
	if opt.MaxQuality == 0 {
		opt.MaxQuality = 92
	}
	if opt.MinQuality == 0 {
		opt.MinQuality = 65
	}
	if opt.MinLong == 0 {
		opt.MinLong = 1400
	}
	if opt.MinQuality < 1 || opt.MaxQuality > 100 || opt.MinQuality > opt.MaxQuality {
		return zero, fmt.Errorf("无效的质量范围")
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	long := max(sw, sh)
	floor := min(long, opt.MinLong)
	w, h := sw, sh
	for attempt := 0; attempt < 24; attempt++ {
		if e := ctx.Err(); e != nil {
			return zero, e
		}
		im := src
		if w != sw || h != sh {
			var e error
			im, e = resizeLanczos(ctx, src, w, h)
			if e != nil {
				return zero, e
			}
		}
		high, e := encodeImage(im, f, opt.MaxQuality)
		if e != nil {
			return zero, e
		}
		if e = ctx.Err(); e != nil {
			return zero, e
		}
		result := func(b []byte, q int) Fitted {
			return Fitted{Data: b, Width: w, Height: h, Quality: q, Format: f, Scaled: w != sw || h != sh}
		}
		if int64(len(high)) <= opt.Limit {
			return result(high, opt.MaxQuality), nil
		}
		low := high
		if f == "jpg" {
			low, e = encodeImage(im, f, opt.MinQuality)
			if e != nil {
				return zero, e
			}
			if int64(len(low)) <= opt.Limit {
				best := low
				bestQ := opt.MinQuality
				lo, hi := opt.MinQuality+1, opt.MaxQuality-1
				for lo <= hi {
					if e = ctx.Err(); e != nil {
						return zero, e
					}
					mid := (lo + hi) / 2
					b, er := encodeImage(im, f, mid)
					if er != nil {
						return zero, er
					}
					if int64(len(b)) <= opt.Limit {
						best = b
						bestQ = mid
						lo = mid + 1
					} else {
						hi = mid - 1
					}
				}
				return result(best, bestQ), nil
			}
		}
		currentLong := max(w, h)
		if currentLong <= floor {
			return zero, errCannotFit
		}
		factor := math.Sqrt(float64(opt.Limit)/float64(len(low))) * 0.94
		factor = math.Max(0.50, math.Min(0.90, factor))
		nextLong := max(floor, int(math.Floor(float64(currentLong)*factor)))
		if nextLong >= currentLong {
			nextLong = currentLong - 1
		}
		scale := float64(nextLong) / float64(long)
		w = max(1, int(math.Round(float64(sw)*scale)))
		h = max(1, int(math.Round(float64(sh)*scale)))
	}
	return zero, errCannotFit
}

type contribution struct {
	index  []int
	weight []float64
}

func sinc(x float64) float64 {
	if math.Abs(x) < 1e-9 {
		return 1
	}
	x *= math.Pi
	return math.Sin(x) / x
}
func weights(old, new int) []contribution {
	out := make([]contribution, new)
	scale := float64(new) / float64(old)
	filter := math.Min(1, scale)
	radius := 3 / filter
	for x := 0; x < new; x++ {
		center := (float64(x)+0.5)/scale - 0.5
		left := int(math.Ceil(center - radius))
		right := int(math.Floor(center + radius))
		total := 0.0
		for j := left; j <= right; j++ {
			d := (center - float64(j)) * filter
			v := sinc(d) * sinc(d/3)
			if math.Abs(d) >= 3 {
				continue
			}
			ix := max(0, min(old-1, j))
			out[x].index = append(out[x].index, ix)
			out[x].weight = append(out[x].weight, v)
			total += v
		}
		for j := range out[x].weight {
			out[x].weight[j] /= total
		}
	}
	return out
}
func byteClamp(v float64) uint8 { return uint8(math.Max(0, math.Min(255, math.Round(v)))) }
func resizeLanczos(ctx context.Context, src *image.RGBA, w, h int) (*image.RGBA, error) {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if w == sw && h == sh {
		return src, nil
	}
	if w < 1 || h < 1 || int64(w)*int64(h) > maxPixels {
		return nil, fmt.Errorf("无效的目标尺寸")
	}
	xw, yw := weights(sw, w), weights(sh, h)
	tmp := image.NewRGBA(image.Rect(0, 0, w, sh))
	for y := 0; y < sh; y++ {
		if y%32 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		for x, c := range xw {
			r, g, b := 0.0, 0.0, 0.0
			for k, ix := range c.index {
				p := y*src.Stride + ix*4
				v := c.weight[k]
				r += float64(src.Pix[p]) * v
				g += float64(src.Pix[p+1]) * v
				b += float64(src.Pix[p+2]) * v
			}
			p := y*tmp.Stride + x*4
			tmp.Pix[p] = byteClamp(r)
			tmp.Pix[p+1] = byteClamp(g)
			tmp.Pix[p+2] = byteClamp(b)
			tmp.Pix[p+3] = 255
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y, c := range yw {
		if y%32 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		for x := 0; x < w; x++ {
			r, g, b := 0.0, 0.0, 0.0
			for k, iy := range c.index {
				p := iy*tmp.Stride + x*4
				v := c.weight[k]
				r += float64(tmp.Pix[p]) * v
				g += float64(tmp.Pix[p+1]) * v
				b += float64(tmp.Pix[p+2]) * v
			}
			p := y*dst.Stride + x*4
			dst.Pix[p] = byteClamp(r)
			dst.Pix[p+1] = byteClamp(g)
			dst.Pix[p+2] = byteClamp(b)
			dst.Pix[p+3] = 255
		}
	}
	return dst, nil
}
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
		return 1
	}
	for p := 2; p+4 <= len(b); {
		if b[p] != 0xff {
			return 1
		}
		marker := b[p+1]
		p += 2
		if marker == 0xda || marker == 0xd9 {
			return 1
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		n := int(binary.BigEndian.Uint16(b[p : p+2]))
		if n < 2 || p+n > len(b) {
			return 1
		}
		d := b[p+2 : p+n]
		if marker == 0xe1 && len(d) >= 14 && string(d[:6]) == "Exif\x00\x00" {
			t := d[6:]
			var order binary.ByteOrder
			if string(t[:2]) == "II" {
				order = binary.LittleEndian
			} else if string(t[:2]) == "MM" {
				order = binary.BigEndian
			} else {
				return 1
			}
			off := int(order.Uint32(t[4:8]))
			if off < 8 || off+2 > len(t) {
				return 1
			}
			count := int(order.Uint16(t[off : off+2]))
			for i := 0; i < count; i++ {
				q := off + 2 + i*12
				if q+12 > len(t) {
					break
				}
				if order.Uint16(t[q:q+2]) == 0x112 && order.Uint16(t[q+2:q+4]) == 3 && order.Uint32(t[q+4:q+8]) == 1 {
					return int(order.Uint16(t[q+8 : q+10]))
				}
			}
		}
		p += n
	}
	return 1
}
func orient(src *image.RGBA, o int) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch o {
			case 2:
				dx = w - 1 - x
			case 3:
				dx = w - 1 - x
				dy = h - 1 - y
			case 4:
				dy = h - 1 - y
			case 5:
				dx = y
				dy = x
			case 6:
				dx = h - 1 - y
				dy = x
			case 7:
				dx = h - 1 - y
				dy = w - 1 - x
			case 8:
				dx = y
				dy = w - 1 - x
			}
			a := y*src.Stride + x*4
			b := dy*dst.Stride + dx*4
			copy(dst.Pix[b:b+4], src.Pix[a:a+4])
		}
	}
	return dst
}
func gifFrameCount(b []byte) int {
	if len(b) < 13 {
		return 0
	}
	p := 13
	if b[10]&128 != 0 {
		p += 3 * (1 << ((b[10] & 7) + 1))
	}
	frames := 0
	skip := func() bool {
		for p < len(b) {
			n := int(b[p])
			p++
			if n == 0 {
				return true
			}
			p += n
			if p > len(b) {
				return false
			}
		}
		return false
	}
	for p < len(b) {
		kind := b[p]
		p++
		switch kind {
		case 0x3b:
			return frames
		case 0x21:
			if p >= len(b) {
				return frames
			}
			p++
			if !skip() {
				return frames
			}
		case 0x2c:
			frames++
			if frames > 1 {
				return frames
			}
			if p+9 > len(b) {
				return frames
			}
			flags := b[p+8]
			p += 9
			if flags&128 != 0 {
				p += 3 * (1 << ((flags & 7) + 1))
			}
			p++
			if !skip() {
				return frames
			}
		default:
			return frames
		}
	}
	return frames
}
