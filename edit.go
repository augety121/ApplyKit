package main

import (
	"context"
	"fmt"
	"image"
	"math/big"
	"strings"
)

// ImageEdit is applied to EXIF-normalized pixels, in this order:
// clockwise rotation, display-axis flips, automatic trim or manual crop.
// Coordinates always refer to the transformed, uncropped image.
type ImageEdit struct {
	SHA256    string          `json:"sha256,omitempty"`
	Rotation  int             `json:"rotation"`
	FlipH     bool            `json:"flipH"`
	FlipV     bool            `json:"flipV"`
	Rect      *NormalizedRect `json:"rect,omitempty"`
	AutoTrim  bool            `json:"autoTrim"`
	Threshold int             `json:"threshold,omitempty"`
	Padding   int             `json:"padding,omitempty"`
}

func (e ImageEdit) changed() bool {
	return e.Rotation != 0 || e.FlipH || e.FlipV || e.AutoTrim || (e.Rect != nil && *e.Rect != fullRect())
}
func (e ImageEdit) validate() error {
	if e.Rotation != 0 && e.Rotation != 90 && e.Rotation != 180 && e.Rotation != 270 {
		return fmt.Errorf("旋转角度必须为 0、90、180 或 270 度")
	}
	if e.Rect != nil {
		if err := e.Rect.validate(); err != nil {
			return err
		}
	}
	if e.Threshold != 0 && (e.Threshold < 200 || e.Threshold > 254) {
		return fmt.Errorf("去白边阈值应在 200～254 之间")
	}
	if e.Padding < 0 || e.Padding > 200 {
		return fmt.Errorf("图片安全边距应在 0～200 像素之间")
	}
	return nil
}
func transformImage(ctx context.Context, src *image.RGBA, edit ImageEdit) (*image.RGBA, error) {
	if err := edit.validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if edit.Rotation == 90 || edit.Rotation == 270 {
		w, h = h, w
	}
	if edit.Rotation == 0 && !edit.FlipH && !edit.FlipV {
		return src, nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		if y%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 0; x < w; x++ {
			tx, ty := x, y
			if edit.FlipH {
				tx = w - 1 - tx
			}
			if edit.FlipV {
				ty = h - 1 - ty
			}
			sx, sy := tx, ty
			switch edit.Rotation {
			case 90:
				sx, sy = ty, b.Dy()-1-tx
			case 180:
				sx, sy = b.Dx()-1-tx, b.Dy()-1-ty
			case 270:
				sx, sy = b.Dx()-1-ty, tx
			}
			i := src.PixOffset(b.Min.X+sx, b.Min.Y+sy)
			j := dst.PixOffset(x, y)
			copy(dst.Pix[j:j+4], src.Pix[i:i+4])
		}
	}
	return dst, nil
}
func applyImageEdit(ctx context.Context, src *image.RGBA, edit ImageEdit, maxEdge int) (*image.RGBA, *CropInfo, error) {
	im, err := transformImage(ctx, src, edit)
	if err != nil {
		return nil, nil, err
	}
	b := im.Bounds()
	keep := b
	note := ""
	mode := "manual"
	blank := false
	if edit.AutoTrim {
		threshold := edit.Threshold
		if threshold == 0 {
			threshold = 250
		}
		det, e := detectWhiteMargins(ctx, im, threshold, edit.Padding, edit.Padding)
		if e != nil {
			return nil, nil, e
		}
		keep, note, blank = det.Bounds, det.Note, det.Blank
		mode = "auto"
	} else if edit.Rect != nil {
		keep, err = edit.Rect.pixels(b)
		if err != nil {
			return nil, nil, err
		}
		note = "按手动框选保留内容"
	}
	var info *CropInfo
	if edit.AutoTrim || edit.Rect != nil {
		info = &CropInfo{Mode: mode, Applied: keep != b, Blank: blank, SourceWidth: b.Dx(), SourceHeight: b.Dy(), Left: keep.Min.X, Top: keep.Min.Y, Width: keep.Dx(), Height: keep.Dy(), Note: note}
		im, err = cropRGBA(im, keep)
		if err != nil {
			return nil, nil, err
		}
	}
	if maxEdge < 0 || (maxEdge > 0 && (maxEdge < 64 || maxEdge > 12000)) {
		return nil, nil, fmt.Errorf("长边尺寸应为 64～12000 像素，或 0（不限制）")
	}
	if maxEdge > 0 && max(im.Bounds().Dx(), im.Bounds().Dy()) > maxEdge {
		w, h := im.Bounds().Dx(), im.Bounds().Dy()
		long := max(w, h)
		im, err = resizeLanczos(ctx, im, max(1, w*maxEdge/long), max(1, h*maxEdge/long))
		if err != nil {
			return nil, nil, err
		}
	}
	return im, info, nil
}
func preparedImage(ctx context.Context, job Job, path string, src *SourceImage) (*image.RGBA, *CropInfo, bool, error) {
	edit := job.Edits[path]
	if edit.changed() {
		hash, err := fileSHA256(ctx, path)
		if err != nil {
			return nil, nil, false, err
		}
		if edit.SHA256 == "" || edit.SHA256 != hash {
			return nil, nil, false, fmt.Errorf("图片自编辑后已更改，或校验信息缺失，请重新打开编辑器保存")
		}
	}
	if job.TrimImages && edit.Rect == nil {
		edit.AutoTrim = true
		edit.Threshold = 250
		edit.Padding = 8
	}
	im, info, err := applyImageEdit(ctx, src.Image, edit, job.MaxEdge)
	changed := edit.changed() || (im != nil && im.Bounds() != src.Image.Bounds())
	return im, info, changed, err
}

// Parse decimal KB/MB without binary floating-point rounding at byte boundaries.
// The limit is exclusive. 2 MB means strictly less than 2,000,000 bytes.
func parseByteLimit(amount, unit string) (int64, error) {
	amount = strings.TrimSpace(amount)
	unit = strings.ToUpper(strings.TrimSpace(unit))
	if len(amount) == 0 || len(amount) > 32 || strings.ContainsAny(amount, "/eE+-") {
		return 0, fmt.Errorf("请输入有效的正数大小，例如 500 KB 或 1.5 MB")
	}
	factor := int64(0)
	switch unit {
	case "KB":
		factor = 1000
	case "MB":
		factor = 1000000
	default:
		return 0, fmt.Errorf("单位应为 KB 或 MB")
	}
	n, ok := new(big.Rat).SetString(amount)
	if !ok || n.Sign() <= 0 {
		return 0, fmt.Errorf("大小必须是正数")
	}
	n.Mul(n, new(big.Rat).SetInt64(factor))
	q := new(big.Int).Quo(n.Num(), n.Denom())
	if !q.IsInt64() || q.Int64() < 1000 || q.Int64() > 100000000 {
		return 0, fmt.Errorf("上限支持 1 KB～100 MB")
	}
	return q.Int64(), nil
}
