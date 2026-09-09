package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Job struct {
	Crop          CropOptions `json:"crop"`
	RenderMaxLong int         `json:"-"`
	Mode          string      `json:"mode"`
	Inputs        []string    `json:"inputs"`
	Output        string      `json:"output"`
	Format        string      `json:"format"`
	Limit         int64       `json:"limit"`
	Profile       string      `json:"profile"`
	DPI           int         `json:"dpi"`
	Pages         string      `json:"pages"`
	Paper         string      `json:"paper"`
	Strict        bool        `json:"strict"`
	MakeZip       bool        `json:"makeZip"`
	Consent       bool        `json:"consent"`
	Progress      string      `json:"progress"`
	Cancel        string      `json:"cancel"`
}
type Record struct {
	Crop    *CropInfo `json:"crop,omitempty"`
	Input   string    `json:"input"`
	Output  string    `json:"output"`
	Status  string    `json:"status"`
	Before  int64     `json:"before"`
	After   int64     `json:"after"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	Page    int       `json:"page,omitempty"`
	Message string    `json:"message"`
}
type Event struct {
	Kind     string  `json:"kind"`
	Message  string  `json:"message,omitempty"`
	Current  int     `json:"current,omitempty"`
	Total    int     `json:"total,omitempty"`
	Record   *Record `json:"record,omitempty"`
	Output   string  `json:"output,omitempty"`
	Success  int     `json:"success,omitempty"`
	Kept     int     `json:"kept,omitempty"`
	Failed   int     `json:"failed,omitempty"`
	Canceled bool    `json:"canceled,omitempty"`
	Elapsed  float64 `json:"elapsed,omitempty"`
}
type Reporter struct {
	mu                    sync.Mutex
	f                     *os.File
	Records               []Record
	Success, Kept, Failed int
}

func (r *Reporter) send(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, e := json.Marshal(ev)
	if e == nil {
		b = append(b, '\n')
		_, _ = r.f.Write(b)
	}
}
func (r *Reporter) add(rec Record) {
	r.Records = append(r.Records, rec)
	switch rec.Status {
	case "saved":
		r.Success++
	case "unchanged":
		r.Kept++
	default:
		r.Failed++
	}
	r.send(Event{Kind: "result", Record: &rec})
}
func (r *Reporter) fail(path string, e error) {
	r.add(Record{Input: path, Status: "failed", Message: e.Error()})
}

func runJob(job Job, assets string) int {
	f, err := os.OpenFile(job.Progress, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return 2
	}
	defer f.Close()
	r := &Reporter{f: f}
	fatal := func(e error) int { r.send(Event{Kind: "fatal", Message: e.Error()}); return 2 }
	if job.Limit < 10000 || job.Limit > 100000000 {
		return fatal(fmt.Errorf("大小上限应在 10KB～100MB 之间"))
	}
	if len(job.Inputs) == 0 || len(job.Inputs) > 200 {
		return fatal(fmt.Errorf("每批应有 1～200 个文件"))
	}
	switch job.Mode {
	case "pdf-images", "image-compress", "images-pdf", "pdf-compress":
	default:
		return fatal(fmt.Errorf("未知处理模式"))
	}
	if err = job.Crop.validate(); err != nil {
		return fatal(err)
	}
	if job.Mode != "pdf-images" && job.Crop.mode() != "none" {
		return fatal(fmt.Errorf("页面裁剪仅用于 PDF 转图片，不会隐式修改其他模式"))
	}
	if job.Mode == "pdf-compress" && !job.Consent {
		return fatal(fmt.Errorf("请先确认扫描版 PDF 重建会丢失文本层和数字签名"))
	}
	if job.DPI != 150 && job.DPI != 200 && job.DPI != 300 {
		job.DPI = 200
	}
	if job.Format == "" {
		job.Format = "jpg"
	}
	if !filepath.IsAbs(job.Output) {
		return fatal(fmt.Errorf("输出目录必须是完整的绝对路径"))
	}
	if err = os.MkdirAll(job.Output, 0700); err != nil {
		return fatal(fmt.Errorf("无法创建输出目录：%w", err))
	}
	output, err := os.MkdirTemp(job.Output, "投递材料_"+time.Now().Format("20060102_150405")+"_")
	if err != nil {
		return fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if job.Cancel != "" {
		go func() {
			t := time.NewTicker(150 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if _, e := os.Stat(job.Cancel); e == nil {
						cancel()
						return
					}
				}
			}
		}()
	}
	begin := time.Now()
	r.send(Event{Kind: "status", Message: "开始处理。原始文件不会被覆盖。", Output: output})
	if job.Mode == "images-pdf" {
		if err = processImagesPDF(ctx, job, output, r); err != nil && !errors.Is(err, context.Canceled) {
			r.fail("图片合并 PDF", err)
		}
	} else {
		for i, input := range job.Inputs {
			if ctx.Err() != nil {
				break
			}
			r.send(Event{Kind: "progress", Current: i, Total: len(job.Inputs), Message: fmt.Sprintf("正在处理 %d / %d：%s", i+1, len(job.Inputs), filepath.Base(input))})
			switch job.Mode {
			case "image-compress":
				err = processImage(ctx, job, input, output, r)
			case "pdf-images":
				err = processPDFImages(ctx, job, input, output, assets, r)
			case "pdf-compress":
				err = processPDFCompress(ctx, job, input, output, assets, r)
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				r.fail(input, err)
			}
		}
	}
	if err = writeReport(job, output, r); err != nil {
		r.fail("处理报告", err)
	}
	if job.MakeZip && ctx.Err() == nil {
		r.send(Event{Kind: "status", Message: "正在整理 ZIP。ZIP 本身不受单张图片大小限制。"})
		if err = zipResults(output); err != nil {
			r.fail("ZIP 打包", err)
		}
	}
	r.send(Event{Kind: "complete", Output: output, Success: r.Success, Kept: r.Kept, Failed: r.Failed, Canceled: ctx.Err() != nil, Elapsed: time.Since(begin).Seconds()})
	return 0
}
func options(job Job, cap int64, format string) FitOptions {
	minQ, minL := 65, 1400
	switch job.Profile {
	case "clear":
		minQ = 78
		minL = 1800
	case "small":
		minQ = 50
		minL = 1000
	}
	return FitOptions{Limit: cap, Format: format, MaxQuality: 92, MinQuality: minQ, MinLong: minL}
}
func budget(job Job) int64 { return (job.Limit * 95) / 100 }
func fmtSize(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.2f MB", float64(n)/1000000)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1000)
}
func fitNote(f Fitted) string {
	s := fmt.Sprintf("%d×%d", f.Width, f.Height)
	if f.Format == "jpg" {
		s += fmt.Sprintf("；JPEG 质量 %d", f.Quality)
	}
	if f.Scaled {
		s += "；已缩小像素尺寸，请预览文字"
	}
	if f.Format == "gif" {
		s += "；GIF 为静态 256 色"
	}
	return s
}
func processImage(ctx context.Context, job Job, input, out string, r *Reporter) error {
	src, err := loadImage(input)
	if err != nil {
		return err
	}
	before := int64(len(src.Bytes))
	if src.Animated {
		return fmt.Errorf("检测到动态 GIF；为避免丢帧，未进行转换或压缩")
	}
	format := normalizeFormat(job.Format)
	if format == "auto" {
		format = src.Format
	}
	cap := min(budget(job), before-1)
	fitted, err := fitImage(ctx, src.Image, options(job, cap, format))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if before < job.Limit {
			path, e := saveUnique(out, safeName(input)+"_原件保留."+src.Format, src.Bytes)
			if e != nil {
				return e
			}
			r.add(Record{Input: input, Output: path, Status: "unchanged", Before: before, After: before, Width: src.Image.Bounds().Dx(), Height: src.Image.Bounds().Dy(), Message: "原文件已在上限以内；无法在清晰度底线内继续缩小，保留原格式和原字节。未冒充压缩成功。"})
			return nil
		}
		return err
	}
	if int64(len(fitted.Data)) >= before || int64(len(fitted.Data)) >= job.Limit {
		return fmt.Errorf("最终大小校验未通过，没有导出")
	}
	ext := fitted.Format
	if job.Format == "jpeg" {
		ext = "jpeg"
	}
	path, err := saveUnique(out, safeName(input)+"_压缩."+ext, fitted.Data)
	if err != nil {
		return err
	}
	r.add(Record{Input: input, Output: path, Status: "saved", Before: before, After: int64(len(fitted.Data)), Width: fitted.Width, Height: fitted.Height, Message: fitNote(fitted)})
	return nil
}
func processImagesPDF(ctx context.Context, job Job, out string, r *Reporter) error {
	totalBefore := int64(0)
	for _, p := range job.Inputs {
		st, e := os.Stat(p)
		if e != nil {
			return e
		}
		if st.Size() > maxInputBytes {
			return fmt.Errorf("输入超过单个 250MB 安全上限")
		}
		totalBefore += st.Size()
	}
	cap := budget(job)
	if job.Strict {
		cap = min(cap, totalBefore-1)
	}
	per := (cap - 4096 - int64(len(job.Inputs))*1100) / int64(len(job.Inputs))
	if per < 1024 {
		return fmt.Errorf("页面过多或目标太小，无法在清晰度底线内合并")
	}
	pages := make([]PDFImage, 0, len(job.Inputs))
	scaled := 0
	for i, p := range job.Inputs {
		if e := ctx.Err(); e != nil {
			return e
		}
		r.send(Event{Kind: "progress", Current: i, Total: len(job.Inputs), Message: fmt.Sprintf("合并顺序 %d / %d：%s", i+1, len(job.Inputs), filepath.Base(p))})
		src, e := loadImage(p)
		if e != nil {
			return fmt.Errorf("%s：%w", filepath.Base(p), e)
		}
		if src.Animated {
			return fmt.Errorf("%s 是动态 GIF；不会静默丢弃后续帧", filepath.Base(p))
		}
		f, e := fitImage(ctx, src.Image, options(job, per, "jpg"))
		if e != nil {
			return fmt.Errorf("第 %d 张：%w", i+1, e)
		}
		if f.Scaled {
			scaled++
		}
		pages = append(pages, PDFImage{JPEG: f.Data, Width: f.Width, Height: f.Height})
	}
	data, e := makePDF(pages, job.Paper)
	if e != nil {
		return e
	}
	if int64(len(data)) >= job.Limit || (job.Strict && int64(len(data)) >= totalBefore) {
		return fmt.Errorf("合并后的 PDF 未通过最终体积校验，没有导出")
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	p, e := saveUnique(out, "投递材料_合并.pdf", data)
	if e != nil {
		return e
	}
	message := fmt.Sprintf("共 %d 页；按队列顺序合并；图像型 PDF（无可搜索文本层）", len(pages))
	if scaled > 0 {
		message += fmt.Sprintf("；%d 张已缩小像素尺寸", scaled)
	}
	r.add(Record{Input: fmt.Sprintf("%d 张图片（合计）", len(pages)), Output: p, Status: "saved", Before: totalBefore, After: int64(len(data)), Message: message})
	return nil
}

type renderEvent struct {
	Kind       string  `json:"kind"`
	Total      int     `json:"total"`
	Selected   int     `json:"selected"`
	Page       int     `json:"page"`
	Path       string  `json:"path"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	PageWidth  float64 `json:"pageWidth"`
	PageHeight float64 `json:"pageHeight"`
	Message    string  `json:"message"`
}

func renderPDF(ctx context.Context, job Job, input, assets string, callback func(renderEvent) error) error {
	st, e := os.Stat(input)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Size() > maxInputBytes {
		return fmt.Errorf("PDF 不是普通文件或超过 250MB 安全上限")
	}
	tmp, e := os.MkdirTemp("", "ApplyKit-render-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	// Arguments are passed without a shell; the PDF password is carried only in the
	// inherited process environment, never a command line, job file, or report.
	c := exec.CommandContext(ctx, powershell(), "-NoLogo", "-NoProfile", "-STA", "-ExecutionPolicy", "RemoteSigned", "-File", filepath.Join(assets, "render.ps1"), "-InputPath", input, "-OutputDir", tmp, "-Dpi", fmt.Sprint(job.DPI), "-Pages", job.Pages, "-CancelPath", job.Cancel, "-MaxLongEdge", fmt.Sprint(renderLongEdge(job)))
	hideCommand(c)
	stdout, e := c.StdoutPipe()
	if e != nil {
		return e
	}
	var stderr bytes.Buffer
	c.Stderr = &stderr
	if e = c.Start(); e != nil {
		return fmt.Errorf("无法启动 Windows PDF 渲染组件：%w", e)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var callbackErr, errorEvent error
	done := false
	for scanner.Scan() {
		line := bytes.TrimPrefix(scanner.Bytes(), []byte{0xef, 0xbb, 0xbf})
		var ev renderEvent
		if e = json.Unmarshal(line, &ev); e != nil {
			continue
		}
		if ev.Kind == "error" {
			errorEvent = errors.New(ev.Message)
			continue
		}
		if ev.Kind == "done" {
			done = true
			continue
		}
		if callbackErr == nil {
			if e = callback(ev); e != nil {
				callbackErr = e
				_ = c.Process.Kill()
			}
		}
	}
	waitErr := c.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if callbackErr != nil {
		return callbackErr
	}
	if errorEvent != nil {
		return errorEvent
	}
	if e = scanner.Err(); e != nil {
		return e
	}
	if waitErr != nil || !done {
		s := strings.TrimSpace(stderr.String())
		if len(s) > 1600 {
			s = s[:1600]
		}
		return fmt.Errorf("Windows PDF 渲染失败。文件可能损坏、受保护，或系统策略限制了组件。%s", s)
	}
	return nil
}

// PDFRenderFunc is injectable so crop, per-page errors and strict budgets can
// be tested without pretending that a Linux test exercised Windows.Data.Pdf.
type PDFRenderFunc func(context.Context, Job, string, string, func(renderEvent) error) error

func renderLongEdge(job Job) int {
	if job.RenderMaxLong > 0 {
		return max(400, min(12000, job.RenderMaxLong))
	}
	return 12000
}
func processPDFImages(ctx context.Context, job Job, input, out, assets string, r *Reporter) error {
	return processPDFImagesWithRenderer(ctx, job, input, out, assets, r, renderPDF)
}
func processPDFImagesWithRenderer(ctx context.Context, job Job, input, out, assets string, r *Reporter, render PDFRenderFunc) error {
	st, e := os.Stat(input)
	if e != nil {
		return e
	}
	before := st.Size()
	if e = job.Crop.validate(); e != nil {
		return e
	}
	var plan *ManualCropPlan
	if job.Crop.mode() == "manual" {
		p, ok := job.Crop.Manual[input]
		if !ok {
			return fmt.Errorf("尚未为 %s 设置手动裁剪，请打开裁剪预览并保存", filepath.Base(input))
		}
		hash, err := fileSHA256(ctx, input)
		if err != nil {
			return err
		}
		if p.SHA256 == "" || p.SHA256 != hash {
			return fmt.Errorf("PDF 自设置裁剪后已更改，或方案校验信息缺失；请重新预览后保存裁剪")
		}
		plan = &p
	}
	var common *NormalizedRect
	if job.Crop.mode() == "uniform" {
		r.send(Event{Kind: "status", Message: "正在分析所选页的统一内容边界（取并集，不取交集）；随后重新渲染导出。"})
		e = render(ctx, job, input, assets, func(ev renderEvent) error {
			if ev.Kind != "page" {
				return nil
			}
			defer os.Remove(ev.Path)
			src, er := loadImage(ev.Path)
			if er != nil {
				return er
			}
			mx, my := marginPixels(job.Crop, ev, src.Image.Bounds(), job.DPI)
			det, er := detectWhiteMargins(ctx, src.Image, job.Crop.threshold(), mx, my)
			if er != nil {
				return er
			}
			if !det.Blank {
				n := normalizeRect(det.Bounds, src.Image.Bounds())
				if common == nil {
					common = &n
				} else {
					u := unionRect(*common, n)
					common = &u
				}
			}
			r.send(Event{Kind: "status", Message: fmt.Sprintf("已分析第 %d 页的白边；空白页不会删除", ev.Page)})
			return nil
		})
		if e != nil {
			return e
		}
	}
	selected := 0
	cap := budget(job)
	format := normalizeFormat(job.Format)
	if format == "auto" {
		format = "jpg"
	}
	return render(ctx, job, input, assets, func(ev renderEvent) error {
		switch ev.Kind {
		case "meta":
			selected = ev.Selected
			if selected < 1 {
				return fmt.Errorf("没有选中页面")
			}
			if job.Strict {
				cap = min(cap, (before-1)/int64(selected))
			}
			r.send(Event{Kind: "status", Message: fmt.Sprintf("%s：选择 %d / %d 页；逐页目标 %s", filepath.Base(input), selected, ev.Total, fmtSize(cap))})
		case "page":
			if selected == 0 {
				return fmt.Errorf("未收到页数信息")
			}
			defer os.Remove(ev.Path)
			src, er := loadImage(ev.Path)
			if er != nil {
				return er
			}
			cropped, info, er := applyPageCrop(ctx, src.Image, ev, job, plan, common)
			if er != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				r.add(Record{Input: input, Page: ev.Page, Status: "failed", Before: before, Message: fmt.Sprintf("第 %d 页裁剪失败：%v", ev.Page, er)})
				return nil
			}
			f, er := fitImage(ctx, cropped, options(job, cap, format))
			if er != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				r.add(Record{Input: input, Page: ev.Page, Status: "failed", Before: before, Crop: info, Message: fmt.Sprintf("第 %d 页：%v", ev.Page, er) + cropNote(info)})
				return nil
			}
			if int64(len(f.Data)) >= job.Limit {
				return fmt.Errorf("第 %d 页未通过大小校验", ev.Page)
			}
			if er = ctx.Err(); er != nil {
				return er
			}
			ext := f.Format
			if job.Format == "jpeg" {
				ext = "jpeg"
			}
			suffix := ""
			if info != nil && info.Applied {
				suffix = "_裁剪"
			}
			path, er := saveUnique(out, fmt.Sprintf("%s_第%03d页%s.%s", safeName(input), ev.Page, suffix, ext), f.Data)
			if er != nil {
				return er
			}
			note := fmt.Sprintf("第 %d 页；%s；相对源 PDF 的体积变化不等同压缩率", ev.Page, fitNote(f)) + cropNote(info)
			r.add(Record{Input: input, Output: path, Status: "saved", Before: before, After: int64(len(f.Data)), Width: f.Width, Height: f.Height, Page: ev.Page, Message: note, Crop: info})
		}
		return nil
	})
}
func processPDFCompress(ctx context.Context, job Job, input, out, assets string, r *Reporter) error {
	st, e := os.Stat(input)
	if e != nil {
		return e
	}
	before := st.Size()
	cap := min(budget(job), before-1)
	var pages []PDFImage
	var per int64
	// Compression always uses all pages; silently dropping pages is not compression.
	job.Pages = ""
	e = renderPDF(ctx, job, input, assets, func(ev renderEvent) error {
		switch ev.Kind {
		case "meta":
			if ev.Selected < 1 {
				return fmt.Errorf("没有 PDF 页面")
			}
			per = (cap - 4096 - int64(ev.Selected)*1100) / int64(ev.Selected)
			if per < 1024 {
				return errCannotFit
			}
		case "page":
			defer os.Remove(ev.Path)
			src, e := loadImage(ev.Path)
			if e != nil {
				return e
			}
			f, e := fitImage(ctx, src.Image, options(job, per, "jpg"))
			if e != nil {
				return e
			}
			pages = append(pages, PDFImage{JPEG: f.Data, Width: f.Width, Height: f.Height, PageWidth: ev.PageWidth, PageHeight: ev.PageHeight})
			r.send(Event{Kind: "status", Message: fmt.Sprintf("正在重建第 %d 页；保留所有页面，不保留文本层 / 签名", ev.Page)})
		}
		return nil
	})
	if e != nil {
		// Only a size-floor failure may fall back to a byte-identical original.
		// Corruption, password failures and cancellation are reported, not hidden.
		if errors.Is(e, errCannotFit) && before < job.Limit {
			data, er := readLimited(input)
			if er != nil {
				return er
			}
			p, er := saveUnique(out, safeName(input)+"_原件保留.pdf", data)
			if er != nil {
				return er
			}
			r.add(Record{Input: input, Output: p, Status: "unchanged", Before: before, After: before, Message: "原 PDF 已在上限以内；重建不能在清晰度底线内变得更小，保留原始 PDF 字节。"})
			return nil
		}
		return e
	}
	data, e := makePDF(pages, "fit")
	if e != nil {
		return e
	}
	if int64(len(data)) >= before || int64(len(data)) >= job.Limit {
		return fmt.Errorf("重建后的 PDF 未变小或仍超限，没有导出")
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	p, e := saveUnique(out, safeName(input)+"_扫描版压缩.pdf", data)
	if e != nil {
		return e
	}
	r.add(Record{Input: input, Output: p, Status: "saved", Before: before, After: int64(len(data)), Message: fmt.Sprintf("共 %d 页；扫描版重建；文本层、表单、超链接和数字签名均不保留", len(pages))})
	return nil
}
func safeName(path string) string {
	s := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var b strings.Builder
	n := 0
	for _, ch := range s {
		if unicode.IsControl(ch) || strings.ContainsRune(`<>:"/\|?*`, ch) {
			ch = '_'
		}
		b.WriteRune(ch)
		n++
		if n >= 64 {
			break
		}
	}
	s = strings.Trim(b.String(), " .")
	if s == "" {
		s = "材料"
	}
	upper := strings.ToUpper(s)
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" || strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT") {
		s = "_" + s
	}
	return s
}
func saveUnique(dir, name string, data []byte) (string, error) {
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("无效的输出文件名")
	}
	temp, e := os.CreateTemp(dir, ".applykit-*.part")
	if e != nil {
		return "", e
	}
	t := temp.Name()
	defer os.Remove(t)
	_, e = temp.Write(data)
	if e == nil {
		e = temp.Sync()
	}
	ce := temp.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return "", e
	}
	base, ext := strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name)
	for i := 0; i < 10000; i++ {
		final := filepath.Join(dir, name)
		if i > 0 {
			final = filepath.Join(dir, fmt.Sprintf("%s__%d%s", base, i+1, ext))
		}
		// Hard-linking is an atomic, non-overwriting publication on NTFS.
		if e = os.Link(t, final); e == nil {
			return final, nil
		}
		if os.IsExist(e) {
			continue
		}
		// FAT / network filesystems may not support hard links. Exclusive create
		// still prevents overwrite; a failed write is removed immediately.
		f, er := os.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(er) {
			continue
		}
		if er != nil {
			return "", er
		}
		_, er = f.Write(data)
		if er == nil {
			er = f.Sync()
		}
		ce = f.Close()
		if er == nil {
			er = ce
		}
		if er != nil {
			os.Remove(final)
			return "", er
		}
		return final, nil
	}
	return "", fmt.Errorf("同名文件过多，无法生成新文件名")
}
func writeReport(job Job, out string, r *Reporter) error {
	summary := struct {
		Version string   `json:"version"`
		Time    string   `json:"time"`
		Mode    string   `json:"mode"`
		Limit   int64    `json:"exclusiveLimitBytes"`
		Target  int64    `json:"targetBytes"`
		Records []Record `json:"records"`
	}{version, time.Now().Format(time.RFC3339), job.Mode, job.Limit, budget(job), r.Records}
	data, e := json.MarshalIndent(summary, "", "  ")
	if e != nil {
		return e
	}
	if _, e = saveUnique(out, "处理报告.json", data); e != nil {
		return e
	}
	var b bytes.Buffer
	b.Write([]byte{0xef, 0xbb, 0xbf})
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"输入", "输出", "状态", "原文件字节", "输出字节", "宽", "高", "PDF页码", "说明"})
	safeCell := func(s string) string {
		if len(s) > 0 && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
			return "'" + s
		}
		return s
	}
	for _, rec := range r.Records {
		_ = w.Write([]string{safeCell(rec.Input), safeCell(rec.Output), rec.Status, fmt.Sprint(rec.Before), fmt.Sprint(rec.After), fmt.Sprint(rec.Width), fmt.Sprint(rec.Height), fmt.Sprint(rec.Page), safeCell(rec.Message)})
	}
	w.Flush()
	if e = w.Error(); e != nil {
		return e
	}
	_, e = saveUnique(out, "处理报告.csv", b.Bytes())
	return e
}
func zipResults(out string) error {
	entries, e := os.ReadDir(out)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(out, ".zip-*.part")
	if e != nil {
		return e
	}
	path := tmp.Name()
	defer os.Remove(path)
	zw := zip.NewWriter(tmp)
	for _, en := range entries {
		if en.IsDir() || strings.HasPrefix(en.Name(), ".") || strings.HasSuffix(en.Name(), ".zip") {
			continue
		}
		f, er := os.Open(filepath.Join(out, en.Name()))
		if er != nil {
			zw.Close()
			tmp.Close()
			return er
		}
		wr, er := zw.Create(en.Name())
		if er == nil {
			_, er = io.Copy(wr, f)
		}
		f.Close()
		if er != nil {
			zw.Close()
			tmp.Close()
			return er
		}
	}
	e = zw.Close()
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	// The per-run directory is new. Existence checks avoid clobbering a user file.
	final := filepath.Join(out, "材料打包_仅用于整理.zip")
	if _, e = os.Stat(final); e == nil {
		return fmt.Errorf("ZIP 文件已存在")
	}
	return os.Rename(path, final)
}
