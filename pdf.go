package main

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// PDFImage is an RGB JPEG, with optional physical page size in PDF points.
type PDFImage struct {
	JPEG                  []byte
	Width, Height         int
	PageWidth, PageHeight float64
}

// makePDF creates an image-only, unencrypted PDF. It deliberately does not claim
// to preserve searchable text, forms, signatures, attachments, or PDF/A status.
func makePDF(images []PDFImage, paper string) ([]byte, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("没有可合并的图片")
	}
	if len(images) > 200 {
		return nil, fmt.Errorf("最多支持 200 页")
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, 3+len(images)*3)
	object := func(id int, data []byte) {
		offsets[id] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", id)
		b.Write(data)
		b.WriteString("\nendobj\n")
	}
	object(1, []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	var kids strings.Builder
	for i := range images {
		fmt.Fprintf(&kids, "%d 0 R ", 3+i*3)
	}
	object(2, []byte(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [ %s] >>", len(images), kids.String())))
	for i, im := range images {
		if im.Width < 1 || im.Height < 1 || len(im.JPEG) < 4 || im.JPEG[0] != 255 || im.JPEG[1] != 216 {
			return nil, fmt.Errorf("第 %d 页不是有效的 JPEG 数据", i+1)
		}
		pw, ph := im.PageWidth, im.PageHeight
		margin := 0.0
		if pw <= 0 || ph <= 0 {
			if paper == "fit" {
				pw = float64(im.Width) * 72 / 200
				ph = float64(im.Height) * 72 / 200
			} else {
				pw = 595.276
				ph = 841.890
				if im.Width > im.Height {
					pw, ph = ph, pw
				}
				margin = 18
			}
		}
		if pw > 14400 || ph > 14400 || pw < 1 || ph < 1 || math.IsNaN(pw) || math.IsNaN(ph) {
			return nil, fmt.Errorf("第 %d 页尺寸超出范围", i+1)
		}
		scale := math.Min((pw-2*margin)/float64(im.Width), (ph-2*margin)/float64(im.Height))
		w, h := float64(im.Width)*scale, float64(im.Height)*scale
		x, y := (pw-w)/2, (ph-h)/2
		pID, cID, iID := 3+i*3, 4+i*3, 5+i*3
		object(pID, []byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.4f %.4f] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>", pw, ph, iID, cID)))
		content := []byte(fmt.Sprintf("q\n%.5f 0 0 %.5f %.5f %.5f cm\n/Im0 Do\nQ\n", w, h, x, y))
		stream := append([]byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(content))), content...)
		stream = append(stream, []byte("endstream")...)
		object(cID, stream)
		header := fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", im.Width, im.Height, len(im.JPEG))
		stream = append([]byte(header), im.JPEG...)
		stream = append(stream, []byte("\nendstream")...)
		object(iID, stream)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return b.Bytes(), nil
}

// A repeated page number is ignored; the first specified order is preserved.
func parsePages(s string, total int) ([]int, error) {
	if total < 1 || total > 200 {
		return nil, fmt.Errorf("PDF 页数应在 1～200 之间")
	}
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "，", ","), "－", "-"))
	if s == "" {
		out := make([]int, total)
		for i := range out {
			out[i] = i + 1
		}
		return out, nil
	}
	out := []int{}
	seen := map[int]bool{}
	for _, piece := range strings.Split(s, ",") {
		part := strings.TrimSpace(piece)
		bounds := strings.Split(part, "-")
		if len(bounds) > 2 {
			return nil, fmt.Errorf("无效页码：%s", part)
		}
		start, e := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if e != nil {
			return nil, fmt.Errorf("无效页码：%s", part)
		}
		end := start
		if len(bounds) == 2 {
			end, e = strconv.Atoi(strings.TrimSpace(bounds[1]))
			if e != nil {
				return nil, fmt.Errorf("无效页码：%s", part)
			}
		}
		if start < 1 || end < start || end > total {
			return nil, fmt.Errorf("页码超出范围：%s（该 PDF 共 %d 页）", part, total)
		}
		for p := start; p <= end; p++ {
			if !seen[p] {
				out = append(out, p)
				seen[p] = true
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有选中 PDF 页面")
	}
	return out, nil
}
