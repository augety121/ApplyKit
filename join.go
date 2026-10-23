package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
)

func validateJoin(job Job) error {
	if !job.JoinImage {
		return nil
	}
	if job.Mode != "pdf-images" && job.Mode != "image-compress" {
		return fmt.Errorf("合成图片仅用于 PDF 转图片或图片处理")
	}
	if job.JoinDirection != "" && job.JoinDirection != "vertical" && job.JoinDirection != "horizontal" {
		return fmt.Errorf("拼接方向应为纵向或横向")
	}
	return nil
}

type joinPart struct {
	path string
	w, h int
}

// Keep only one decoded source at a time. Prepared parts live in a private,
// bounded temporary directory; no partial result is published on failure.
func processJoinedImage(ctx context.Context, job Job, out, assets string, r *Reporter, renderer PDFRenderFunc) error {
	if err := validateJoin(job); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(out, ".join-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	parts := []joinPart{}
	var diskBytes, before int64
	collect := func(im *image.RGBA) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(parts) >= 100 {
			return fmt.Errorf("单张图片最多拼接 100 页或张，请分组导出")
		}
		w, h := im.Bounds().Dx(), im.Bounds().Dy()
		if max(w, h) > 2400 {
			im, err = resizeLanczos(ctx, im, max(1, w*2400/max(w, h)), max(1, h*2400/max(w, h)))
			if err != nil {
				return err
			}
		}
		data, e := encodeImage(im, "png", 92)
		if e != nil {
			return e
		}
		diskBytes += int64(len(data))
		if diskBytes > 512*1024*1024 {
			return fmt.Errorf("拼接中间材料超过 512 MB，请减少本组材料")
		}
		p := filepath.Join(tmp, fmt.Sprintf("%03d.png", len(parts)))
		if e = os.WriteFile(p, data, 0600); e != nil {
			return e
		}
		parts = append(parts, joinPart{p, im.Bounds().Dx(), im.Bounds().Dy()})
		r.send(Event{Kind: "status", Message: fmt.Sprintf("已准备 %d 页/张，完成后合成一张图片", len(parts))})
		return nil
	}
	for _, input := range job.Inputs {
		if err = ctx.Err(); err != nil {
			return err
		}
		st, e := os.Stat(input)
		if e != nil {
			return e
		}
		before += st.Size()
		if job.Mode == "pdf-images" {
			partJob := job
			partJob.Strict = false
			partJob.MaxEdge = 0
			partJob.RenderMaxLong = 2400
			partJob.collectPage = collect
			if e = processPDFImagesWithRenderer(ctx, partJob, input, tmp, assets, r, renderer); e != nil {
				return e
			}
		} else {
			src, e := loadImageSized(ctx, input, 0)
			if e != nil {
				return e
			}
			if src.Animated {
				return fmt.Errorf("%s 是动态 GIF，不能静默丢弃帧后拼接", filepath.Base(input))
			}
			partJob := job
			partJob.MaxEdge = 0
			im, _, _, e := preparedImage(ctx, partJob, input, src)
			if e != nil {
				return e
			}
			if e = collect(im); e != nil {
				return e
			}
		}
	}
	if len(parts) == 0 {
		return fmt.Errorf("没有可合成的页面或图片")
	}
	if job.MaxEdge > 0 && len(parts) > job.MaxEdge {
		return fmt.Errorf("最长边过小，无法保留全部材料；请提高最长边或分组导出")
	}
	rects, w, h := joinLayout(parts, job.JoinDirection, job.MaxEdge)
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for i, p := range parts {
		if err = ctx.Err(); err != nil {
			return err
		}
		src, e := loadImageSized(ctx, p.path, 0)
		if e != nil {
			return e
		}
		rect := rects[i]
		im, e := resizeLanczos(ctx, src.Image, rect.Dx(), rect.Dy())
		if e != nil {
			return e
		}
		draw.Draw(canvas, rect, im, im.Bounds().Min, draw.Src)
	}
	format := normalizeFormat(job.Format)
	if format == "auto" {
		format = "jpg"
	}
	cap := budget(job)
	if job.Strict {
		cap = min(cap, before-1)
	}
	fitted, e := fitImage(ctx, canvas, options(job, cap, format))
	if e != nil {
		return e
	}
	if int64(len(fitted.Data)) >= job.Limit || (job.Strict && int64(len(fitted.Data)) >= before) {
		return fmt.Errorf("合成图片未通过整张大小校验，没有导出")
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	ext := fitted.Format
	if job.Format == "jpeg" {
		ext = "jpeg"
	}
	path, e := saveUnique(out, "投递材料_合成一张."+ext, fitted.Data)
	if e != nil {
		return e
	}
	direction := "纵向"
	if job.JoinDirection == "horizontal" {
		direction = "横向"
	}
	r.add(Record{Input: fmt.Sprintf("%d 页/张材料（合计）", len(parts)), Output: path, Status: "saved", Before: before, After: int64(len(fitted.Data)), Width: fitted.Width, Height: fitted.Height, Message: fmt.Sprintf("共 %d 页/张，按列表及 PDF 页码顺序%s合成一张；上限针对整张；等比适配，未删除页面；%s；请放大检查文字，材料过多时分组导出", len(parts), direction, fitNote(fitted))})
	return nil
}

// Normalize the cross-axis, preserve aspect ratio, and bound both dimensions
// and canvas pixels before allocation. No source can disappear after rounding.
func joinLayout(parts []joinPart, direction string, maxEdge int) ([]image.Rectangle, int, int) {
	horizontal := direction == "horizontal"
	cross := 1
	for _, p := range parts {
		c := p.w
		if horizontal {
			c = p.h
		}
		cross = max(cross, c)
	}
	cross = min(cross, 2400)
	lengths := make([]float64, len(parts))
	total := 0.0
	for i, p := range parts {
		c, l := p.w, p.h
		if horizontal {
			c, l = p.h, p.w
		}
		lengths[i] = float64(l) * float64(cross) / float64(c)
		total += lengths[i]
	}
	edge := 12000
	if maxEdge > 0 {
		edge = min(edge, maxEdge)
	}
	scale := math.Min(1, math.Min(float64(edge)/math.Max(float64(cross), total), math.Sqrt(float64(maxPixels)/(float64(cross)*total))))
	// Reserve one pixel per item so rounding cannot exceed the long-edge cap.
	if total*scale+float64(len(parts)) > float64(edge) {
		scale = math.Max(0, (float64(edge)-float64(len(parts)))/total)
	}
	c := max(1, int(float64(cross)*scale))
	pos := 0
	rects := make([]image.Rectangle, len(parts))
	for i, l := range lengths {
		n := max(1, int(l*scale))
		if horizontal {
			rects[i] = image.Rect(pos, 0, pos+n, c)
		} else {
			rects[i] = image.Rect(0, pos, c, pos+n)
		}
		pos += n
	}
	if horizontal {
		return rects, pos, c
	}
	return rects, c, pos
}
