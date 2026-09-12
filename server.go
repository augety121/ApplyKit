package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var webFiles embed.FS

// Synthetic examples contain no personal data and follow the normal upload path.
//go:embed samples/crop-demo.pdf samples/text-demo.png
var exampleFiles embed.FS

const maxSessionBytes int64 = 2 * 1024 * 1024 * 1024

type FileItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Bytes  int64  `json:"bytes"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	SHA256 string `json:"sha256"`
	Path   string `json:"-"`
}
type Preview struct {
	FileID       string  `json:"-"`
	Key          string  `json:"key"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	SourceWidth  int     `json:"sourceWidth"`
	SourceHeight int     `json:"sourceHeight"`
	Page         int     `json:"page"`
	Total        int     `json:"total"`
	PageWidth    float64 `json:"pageWidth"`
	PageHeight   float64 `json:"pageHeight"`
	Path         string  `json:"-"`
}
type sequencedEvent struct {
	Seq int `json:"seq"`
	Event
}
type runningJob struct {
	ID       string             `json:"id"`
	Request  Job                `json:"-"`
	Cancel   context.CancelFunc `json:"-"`
	Events   []sequencedEvent   `json:"-"`
	Records  []Record           `json:"-"`
	Done     bool               `json:"done"`
	Output   string             `json:"output"`
	Started  time.Time          `json:"started"`
	InputIDs []string           `json:"inputIds"`
	Limit    int64              `json:"limit"`
}
type PreferenceMode struct {
	Amount     string   `json:"amount"`
	Unit       string   `json:"unit"`
	Format     string   `json:"format"`
	Profile    string   `json:"profile"`
	DPI        int      `json:"dpi"`
	Paper      string   `json:"paper"`
	CropMode   string   `json:"cropMode"`
	MarginMM   *float64 `json:"marginMM,omitempty"`
	MaxEdge    int      `json:"maxEdge"`
	Strict     bool     `json:"strict"`
	MakeZip    bool     `json:"makeZip"`
	TrimImages bool     `json:"trimImages"`
}
type SizePreset struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
	Unit   string `json:"unit"`
}
type Preferences struct {
	Theme   string                    `json:"theme"`
	Mode    string                    `json:"mode"`
	Output  string                    `json:"output"`
	Tools   map[string]PreferenceMode `json:"tools"`
	Presets []SizePreset              `json:"presets"`
}
type AppServer struct {
	mu                                   sync.Mutex
	uploadMu                             sync.Mutex
	token, host, root, assets, prefsPath string
	files                                map[string]*FileItem
	previews                             map[string]Preview
	previewByKey                         map[string]Preview
	jobs                                 map[string]*runningJob
	active                               string
	renderer                             PDFRenderFunc
	compute                              chan struct{}
	stop                                 context.CancelFunc
	prefs                                Preferences
	leases                               int
	lastLease                            time.Time
	closing                              bool
}

func randomID() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func localDataRoot() string {
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		return filepath.Join(base, "ApplyKit")
	}
	home, e := os.UserConfigDir()
	if e != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, "ApplyKit")
}
func defaultOutputDir() string {
	home, e := os.UserHomeDir()
	if e != nil {
		return filepath.Join(os.TempDir(), "ApplyKit-exports")
	}
	return filepath.Join(home, "Downloads", "ApplyKit")
}
func newAppServer(dataDir string, renderer PDFRenderFunc) (*AppServer, error) {
	if dataDir == "" {
		dataDir = localDataRoot()
	}
	if e := os.MkdirAll(dataDir, 0700); e != nil {
		return nil, e
	}
	root, e := os.MkdirTemp(dataDir, "session-")
	if e != nil {
		return nil, e
	}
	assets, e := assetDir()
	if e != nil {
		os.RemoveAll(root)
		return nil, e
	}
	s := &AppServer{token: randomID(), root: root, assets: assets, prefsPath: filepath.Join(dataDir, "preferences-v2.json"), files: map[string]*FileItem{}, previews: map[string]Preview{}, previewByKey: map[string]Preview{}, jobs: map[string]*runningJob{}, renderer: renderer, compute: make(chan struct{}, 1), lastLease: time.Now()}
	s.prefs = Preferences{Theme: "light", Mode: "pdf-images", Output: defaultOutputDir(), Tools: map[string]PreferenceMode{}, Presets: []SizePreset{}}
	if b, e := os.ReadFile(s.prefsPath); e == nil && len(b) < 32768 {
		var p Preferences
		if json.Unmarshal(b, &p) == nil && validatePrefs(p) == nil {
			s.prefs = p
		}
	}
	return s, nil
}
func (s *AppServer) Close() {
	s.mu.Lock()
	s.closing = true
	for _, j := range s.jobs {
		if !j.Done {
			j.Cancel()
		}
	}
	s.mu.Unlock()
	// Jobs own a cancellable context. Give in-flight encoders a bounded cleanup
	// window before deleting session-only sources. Exported files are untouched.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		busy := false
		for _, j := range s.jobs {
			busy = busy || !j.Done
		}
		s.mu.Unlock()
		if !busy {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = os.RemoveAll(s.root)
}
func apiError(w http.ResponseWriter, status int, err error) {
	sendJSON(w, status, map[string]string{"error": err.Error()})
}
func sendJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decodeBody(w http.ResponseWriter, r *http.Request, out any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return fmt.Errorf("请求参数无效：%w", e)
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("请求只能包含一个 JSON 对象")
	}
	return nil
}
func (s *AppServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if recover() != nil {
			apiError(w, 500, fmt.Errorf("内部处理异常；原材料未修改，请重试或查看诊断"))
		}
	}()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; connect-src 'self'; worker-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	if s.host != "" && r.Host != s.host {
		apiError(w, 403, fmt.Errorf("无效的本地主机"))
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != "http://"+r.Host {
		apiError(w, 403, fmt.Errorf("不允许来自其他网站的请求"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
			apiError(w, 401, fmt.Errorf("会话无效，请通过 ApplyKit.exe 重新打开软件"))
			return
		}
		s.routeAPI(w, r)
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	sub, _ := fs.Sub(webFiles, "web")
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if strings.Contains(p, "..") {
		http.NotFound(w, r)
		return
	}
	b, e := fs.ReadFile(sub, p)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	switch filepath.Ext(p) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js", ".mjs":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	http.ServeContent(w, r, p, time.Time{}, bytes.NewReader(b))
}
func (s *AppServer) routeAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	switch {
	case (path == "example/crop-demo.pdf" || path == "example/text-demo.png") && r.Method == "GET":
		name := strings.TrimPrefix(path, "example/")
		b, err := exampleFiles.ReadFile("samples/" + name)
		if err != nil {
			apiError(w, 500, err)
			return
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
	case path == "config" && r.Method == "GET":
		s.mu.Lock()
		p := s.prefs
		s.mu.Unlock()
		sendJSON(w, 200, map[string]any{"version": version, "platform": runtime.GOOS, "preferences": p, "maxInputBytes": maxInputBytes, "maxPixels": maxPixels, "maxFiles": 200, "renderer": rendererName()})
	case path == "prefs" && r.Method == "PUT":
		var p Preferences
		if e := decodeBody(w, r, &p, 32768); e != nil {
			apiError(w, 400, e)
			return
		}
		if e := validatePrefs(p); e != nil {
			apiError(w, 400, e)
			return
		}
		b, _ := json.MarshalIndent(p, "", "  ")
		s.mu.Lock()
		e := os.WriteFile(s.prefsPath, b, 0600)
		if e == nil {
			s.prefs = p
		}
		s.mu.Unlock()
		if e != nil {
			apiError(w, 500, e)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
	case path == "files" && r.Method == "GET":
		s.mu.Lock()
		out := []FileItem{}
		for _, f := range s.files {
			out = append(out, *f)
		}
		s.mu.Unlock()
		sendJSON(w, 200, out)
	case path == "upload" && r.Method == "POST":
		s.upload(w, r)
	case strings.HasPrefix(path, "files/") && r.Method == "DELETE":
		s.removeFile(w, r, strings.TrimPrefix(path, "files/"))
	case strings.HasPrefix(path, "preview/") && r.Method == "POST":
		s.preview(w, r, strings.TrimPrefix(path, "preview/"))
	case strings.HasPrefix(path, "blob/") && r.Method == "GET":
		key := strings.TrimPrefix(path, "blob/")
		s.mu.Lock()
		p, ok := s.previewByKey[key]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		serveLocalFile(w, r, p.Path, false)
	case path == "autocrop" && r.Method == "POST":
		s.autoCrop(w, r)
	case path == "jobs" && r.Method == "POST":
		s.startJob(w, r)
	case strings.HasPrefix(path, "jobs/") && r.Method == "GET":
		s.jobEvents(w, r, strings.TrimPrefix(path, "jobs/"))
	case strings.HasPrefix(path, "cancel/") && r.Method == "POST":
		id := strings.TrimPrefix(path, "cancel/")
		s.mu.Lock()
		j := s.jobs[id]
		if j != nil && !j.Done {
			j.Cancel()
		}
		s.mu.Unlock()
		sendJSON(w, 200, map[string]bool{"ok": true})
	case strings.HasPrefix(path, "result/") && r.Method == "GET":
		s.result(w, r, strings.TrimPrefix(path, "result/"))
	case path == "folder" && r.Method == "POST":
		s.mu.Lock()
		p := s.prefs.Output
		s.mu.Unlock()
		out, e := chooseFolder(r.Context(), p, s.assets)
		if e != nil {
			apiError(w, 400, e)
			return
		}
		sendJSON(w, 200, map[string]string{"path": out})
	case path == "open" && r.Method == "POST":
		s.openResult(w, r)
	case path == "lease" && r.Method == "GET":
		s.lease(w, r)
	case path == "diagnostics" && r.Method == "GET":
		sendJSON(w, 200, map[string]any{"version": version, "platform": runtime.GOOS, "arch": runtime.GOARCH, "renderer": rendererName(), "goVersion": runtime.Version(), "runtimeChecks": runtimeChecks(), "networkBinding": "127.0.0.1 only", "includesPersonalFiles": false})
	default:
		http.NotFound(w, r)
	}
}
func validatePrefs(p Preferences) error {
	if p.Theme != "light" && p.Theme != "dark" {
		return fmt.Errorf("主题无效")
	}
	if !validMode(p.Mode) {
		return fmt.Errorf("工具模式无效")
	}
	if !filepath.IsAbs(p.Output) || len(p.Output) > 2000 {
		return fmt.Errorf("保存位置必须为完整路径")
	}
	if len(p.Tools) > 4 || len(p.Presets) > 12 {
		return fmt.Errorf("预设过多")
	}
	for k, v := range p.Tools {
		if !validMode(k) {
			return fmt.Errorf("工具模式无效")
		}
		if _, e := parseByteLimit(v.Amount, v.Unit); e != nil {
			return e
		}
		if v.MarginMM != nil && (*v.MarginMM < 0 || *v.MarginMM > 20) {
			return fmt.Errorf("安全边距为 0～20 毫米")
		}
		if len(v.Format) > 12 || len(v.Profile) > 16 || len(v.CropMode) > 16 || len(v.Paper) > 12 {
			return fmt.Errorf("预设参数过长")
		}
	}
	for _, v := range p.Presets {
		if len(v.Name) == 0 || len([]rune(v.Name)) > 24 {
			return fmt.Errorf("预设名称为 1～24 字")
		}
		if _, e := parseByteLimit(v.Amount, v.Unit); e != nil {
			return e
		}
	}
	return nil
}
func validMode(s string) bool {
	return s == "pdf-images" || s == "image-compress" || s == "images-pdf" || s == "pdf-compress"
}
func (s *AppServer) upload(w http.ResponseWriter, r *http.Request) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	name := r.URL.Query().Get("name")
	ext := strings.ToLower(filepath.Ext(name))
	kind := "image"
	switch ext {
	case ".pdf":
		kind = "pdf"
	case ".jpg", ".jpeg", ".png", ".gif":
	default:
		apiError(w, 400, fmt.Errorf("支持 PDF、JPG、JPEG、PNG、静态 GIF"))
		return
	}
	s.mu.Lock()
	count := len(s.files)
	total := int64(0)
	busy := s.active != ""
	for _, f := range s.files {
		total += f.Bytes
	}
	s.mu.Unlock()
	if busy {
		apiError(w, 409, fmt.Errorf("任务处理中，请完成或停止后再添加材料"))
		return
	}
	if count >= 200 {
		apiError(w, 400, fmt.Errorf("每个工作区最多 200 个文件"))
		return
	}
	id := randomID()
	dir := filepath.Join(s.root, "inputs", id)
	if e := os.MkdirAll(dir, 0700); e != nil {
		apiError(w, 500, e)
		return
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()
	clean := safeName(name) + ext
	path := filepath.Join(dir, clean)
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		apiError(w, 500, e)
		return
	}
	h := sha256.New()
	r.Body = http.MaxBytesReader(w, r.Body, maxInputBytes)
	n, e := io.Copy(io.MultiWriter(f, h), r.Body)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil || n == 0 {
		apiError(w, 400, fmt.Errorf("读取失败，或单文件超过 250 MB；请检查材料"))
		return
	}
	if total+n > maxSessionBytes {
		apiError(w, 400, fmt.Errorf("工作区累计材料超过 2 GB，请分批处理"))
		return
	}
	hash := hex.EncodeToString(h.Sum(nil))
	s.mu.Lock()
	for _, old := range s.files {
		if old.SHA256 == hash {
			s.mu.Unlock()
			sendJSON(w, 200, map[string]any{"file": old, "duplicate": true})
			return
		}
	}
	s.mu.Unlock()
	item := &FileItem{ID: id, Name: clean, Kind: kind, Bytes: n, SHA256: hash, Path: path}
	if kind == "pdf" {
		ff, _ := os.Open(path)
		head := make([]byte, 1024)
		nr, _ := ff.Read(head)
		ff.Close()
		if !bytes.Contains(head[:nr], []byte("%PDF-")) {
			apiError(w, 400, fmt.Errorf("文件内容不是可识别的 PDF"))
			return
		}
	} else {
		data, e := readLimited(path)
		if e != nil {
			apiError(w, 400, e)
			return
		}
		cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
		if e != nil {
			apiError(w, 400, fmt.Errorf("图片已损坏或格式不支持"))
			return
		}
		if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
			apiError(w, 400, fmt.Errorf("图片超过 3200 万像素安全上限"))
			return
		}
		if format == "gif" && gifFrameCount(data) > 1 {
			apiError(w, 400, fmt.Errorf("动态 GIF 不会静默丢帧；请使用静态图片"))
			return
		}
		item.Width, item.Height = cfg.Width, cfg.Height
		if format == "jpeg" {
			o := exifOrientation(data)
			if o >= 5 && o <= 8 {
				item.Width, item.Height = item.Height, item.Width
			}
		}
	}
	s.mu.Lock()
	s.files[id] = item
	s.mu.Unlock()
	ok = true
	sendJSON(w, 201, map[string]any{"file": item, "duplicate": false})
}
func (s *AppServer) removeFile(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	if s.active != "" {
		s.mu.Unlock()
		apiError(w, 409, fmt.Errorf("正在处理，暂不能移除材料"))
		return
	}
	f := s.files[id]
	delete(s.files, id)
	s.mu.Unlock()
	if f != nil {
		_ = os.RemoveAll(filepath.Dir(f.Path))
	}
	sendJSON(w, 200, map[string]bool{"ok": true})
}
func (s *AppServer) file(id string) (FileItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.files[id]
	if f == nil {
		return FileItem{}, fmt.Errorf("材料已移除，请重新添加")
	}
	return *f, nil
}
func (s *AppServer) acquire(ctx context.Context) error {
	select {
	case s.compute <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *AppServer) preparePreview(ctx context.Context, id string, page int, password string) (Preview, error) {
	f, e := s.file(id)
	if e != nil {
		return Preview{}, e
	}
	if page < 1 || page > 200 {
		return Preview{}, fmt.Errorf("页码应在 1～200 之间")
	}
	sum := sha256.Sum256([]byte(password))
	cacheKey := fmt.Sprintf("%s-%d-%x", id, page, sum[:8])
	if e = s.acquire(ctx); e != nil {
		return Preview{}, e
	}
	defer func() { <-s.compute }()
	s.mu.Lock()
	cached, ok := s.previews[cacheKey]
	s.mu.Unlock()
	if ok {
		return cached, nil
	}
	p := Preview{Key: randomID(), FileID: id, Page: page, Total: 1}
	var im *image.RGBA
	if f.Kind == "image" {
		src, e := loadImage(f.Path)
		if e != nil {
			return p, e
		}
		if src.Animated {
			return p, fmt.Errorf("不支持动态 GIF")
		}
		im = src.Image
		p.SourceWidth, p.SourceHeight = im.Bounds().Dx(), im.Bounds().Dy()
	} else {
		job := Job{DPI: 110, RenderMaxLong: 1600, Pages: strconv.Itoa(page), Password: password}
		e = s.renderer(ctx, job, f.Path, s.assets, func(ev renderEvent) error {
			if ev.Kind == "meta" {
				p.Total = ev.Total
				return nil
			}
			if ev.Kind != "page" {
				return nil
			}
			defer os.Remove(ev.Path)
			src, err := loadImage(ev.Path)
			if err != nil {
				return err
			}
			im = src.Image
			p.PageWidth, p.PageHeight = ev.PageWidth, ev.PageHeight
			p.SourceWidth, p.SourceHeight = im.Bounds().Dx(), im.Bounds().Dy()
			return nil
		})
		if e != nil {
			return p, e
		}
		if im == nil {
			return p, fmt.Errorf("未生成页面预览")
		}
	}
	long := max(im.Bounds().Dx(), im.Bounds().Dy())
	if long > 1600 {
		im, e = resizeLanczos(ctx, im, max(1, im.Bounds().Dx()*1600/long), max(1, im.Bounds().Dy()*1600/long))
		if e != nil {
			return p, e
		}
	}
	p.Width, p.Height = im.Bounds().Dx(), im.Bounds().Dy()
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if e = enc.Encode(&buf, im); e != nil {
		return p, e
	}
	if e = ctx.Err(); e != nil {
		return p, e
	}
	dir := filepath.Join(s.root, "previews")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return p, e
	}
	p.Path = filepath.Join(dir, p.Key+".png")
	if e = os.WriteFile(p.Path, buf.Bytes(), 0600); e != nil {
		return p, e
	}
	s.mu.Lock()
	s.previews[cacheKey] = p
	s.previewByKey[p.Key] = p
	s.mu.Unlock()
	return p, nil
}
func (s *AppServer) preview(w http.ResponseWriter, r *http.Request, id string) {
	var q struct {
		Page     int    `json:"page"`
		Password string `json:"password"`
	}
	if e := decodeBody(w, r, &q, 4096); e != nil {
		apiError(w, 400, e)
		return
	}
	if q.Page == 0 {
		q.Page = 1
	}
	p, e := s.preparePreview(r.Context(), id, q.Page, q.Password)
	if e != nil {
		apiError(w, 400, e)
		return
	}
	sendJSON(w, 200, p)
}
func (s *AppServer) autoCrop(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID         string    `json:"id"`
		PreviewKey string    `json:"previewKey"`
		Edit       ImageEdit `json:"edit"`
		MarginMM   float64   `json:"marginMM"`
	}
	if e := decodeBody(w, r, &q, 16384); e != nil {
		apiError(w, 400, e)
		return
	}
	if e := q.Edit.validate(); e != nil {
		apiError(w, 400, e)
		return
	}
	if e := s.acquire(r.Context()); e != nil {
		return
	}
	defer func() { <-s.compute }()
	f, e := s.file(q.ID)
	if e != nil {
		apiError(w, 404, e)
		return
	}
	path := f.Path
	mx, my := q.Edit.Padding, q.Edit.Padding
	if f.Kind == "pdf" {
		s.mu.Lock()
		p, ok := s.previewByKey[q.PreviewKey]
		s.mu.Unlock()
		if !ok || p.FileID != q.ID {
			apiError(w, 400, fmt.Errorf("请先加载页面预览"))
			return
		}
		if q.MarginMM < 0 || q.MarginMM > 20 {
			apiError(w, 400, fmt.Errorf("安全边距为 0～20 毫米"))
			return
		}
		path = p.Path
		mx, my = marginPixels(CropOptions{MarginMM: q.MarginMM}, renderEvent{PageWidth: p.PageWidth, PageHeight: p.PageHeight}, image.Rect(0, 0, p.Width, p.Height), 110)
	}
	src, e := loadImage(path)
	if e != nil {
		apiError(w, 400, e)
		return
	}
	im, e := transformImage(r.Context(), src.Image, q.Edit)
	if e != nil {
		apiError(w, 400, e)
		return
	}
	threshold := q.Edit.Threshold
	if threshold == 0 {
		threshold = 250
	}
	d, e := detectWhiteMargins(r.Context(), im, threshold, mx, my)
	if e != nil {
		apiError(w, 400, e)
		return
	}
	sendJSON(w, 200, map[string]any{"rect": normalizeRect(d.Bounds, im.Bounds()), "blank": d.Blank, "note": d.Note})
}
func (s *AppServer) startJob(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Job
		Amount        string `json:"amount"`
		Unit          string `json:"unit"`
		PasswordInput string `json:"password"`
	}
	if e := decodeBody(w, r, &q, 512*1024); e != nil {
		apiError(w, 400, e)
		return
	}
	limit, e := parseByteLimit(q.Amount, q.Unit)
	if e != nil {
		apiError(w, 400, e)
		return
	}
	if !validMode(q.Mode) || len(q.Inputs) == 0 || len(q.Inputs) > 200 {
		apiError(w, 400, fmt.Errorf("请选择有效工具与 1～200 个材料"))
		return
	}
	if e = q.Crop.validate(); e != nil {
		apiError(w, 400, e)
		return
	}
	if len(q.PasswordInput) > 1024 || len(q.Pages) > 2000 {
		apiError(w, 400, fmt.Errorf("密码或页码参数过长"))
		return
	}
	if q.Mode == "pdf-compress" && !q.Consent {
		apiError(w, 400, fmt.Errorf("请先确认扫描重建会丢失文本层、表单与数字签名"))
		return
	}
	if q.Mode != "pdf-images" && q.Crop.mode() != "none" {
		apiError(w, 400, fmt.Errorf("PDF 裁剪只能用于 PDF 转图片"))
		return
	}
	if q.MaxEdge < 0 || (q.MaxEdge > 0 && (q.MaxEdge < 64 || q.MaxEdge > 12000)) {
		apiError(w, 400, fmt.Errorf("长边应为 64～12000 像素，或 0"))
		return
	}
	allowedFormats := map[string]bool{"jpg": true, "jpeg": true, "png": true, "gif": true, "auto": true}
	if !allowedFormats[q.Format] {
		apiError(w, 400, fmt.Errorf("输出格式无效"))
		return
	}
	s.mu.Lock()
	if s.active != "" || s.closing {
		s.mu.Unlock()
		apiError(w, 409, fmt.Errorf("已有任务正在处理，请等待或停止"))
		return
	}
	ids := append([]string{}, q.Inputs...)
	paths := []string{}
	manual := map[string]ManualCropPlan{}
	edits := map[string]ImageEdit{}
	seen := map[string]bool{}
	for _, id := range ids {
		f := s.files[id]
		if f == nil || seen[id] {
			s.mu.Unlock()
			apiError(w, 400, fmt.Errorf("材料不存在或在队列重复"))
			return
		}
		seen[id] = true
		needsPDF := q.Mode == "pdf-images" || q.Mode == "pdf-compress"
		if (f.Kind == "pdf") != needsPDF {
			s.mu.Unlock()
			apiError(w, 400, fmt.Errorf("材料类型与当前工具不匹配"))
			return
		}
		paths = append(paths, f.Path)
		if p, ok := q.Crop.Manual[id]; ok {
			manual[f.Path] = p
		}
		if edit, ok := q.Edits[id]; ok {
			if e = edit.validate(); e != nil {
				s.mu.Unlock()
				apiError(w, 400, e)
				return
			}
			edits[f.Path] = edit
		}
	}
	if q.Output == "" {
		q.Output = s.prefs.Output
	}
	if !filepath.IsAbs(q.Output) || len(q.Output) > 2000 {
		s.mu.Unlock()
		apiError(w, 400, fmt.Errorf("保存位置必须为完整路径"))
		return
	}
	// Requests may use only registered session files. Job/report/cancel paths
	// supplied by a client are deliberately ignored.
	job := q.Job
	job.Limit = limit
	job.Inputs = paths
	job.Crop.Manual = manual
	job.Edits = edits
	job.Progress = ""
	job.Cancel = ""
	job.Password = q.PasswordInput
	id := randomID()
	ctx, cancel := context.WithCancel(context.Background())
	j := &runningJob{ID: id, Request: job, Cancel: cancel, InputIDs: ids, Limit: limit, Started: time.Now(), Events: []sequencedEvent{}, Records: []Record{}}
	s.jobs[id] = j
	s.active = id
	s.mu.Unlock()
	go func() {
		defer cancel()
		defer func() {
			if e := recover(); e != nil {
				s.pushEvent(id, Event{Kind: "fatal", Message: "处理异常已中止；原材料未覆盖，请分批重试"})
			}
			s.mu.Lock()
			j.Done = true
			j.Request.Password = ""
			if s.active == id {
				s.active = ""
			}
			s.mu.Unlock()
		}()
		runJobContext(ctx, job, s.assets, func(ev Event) { s.pushEvent(id, ev) }, s.renderer)
	}()
	sendJSON(w, 201, map[string]any{"id": id, "limit": limit, "target": budget(job)})
}
func (s *AppServer) pushEvent(id string, ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	if j == nil {
		return
	}
	if ev.Output != "" {
		j.Output = ev.Output
	}
	if ev.Record != nil {
		rec := *ev.Record
		rec.ID = strconv.Itoa(len(j.Records))
		for fid, f := range s.files {
			if f.Path == rec.Input {
				rec.InputID = fid
				break
			}
		}
		j.Records = append(j.Records, rec)
		ev.Record = &rec
	}
	j.Events = append(j.Events, sequencedEvent{Seq: len(j.Events) + 1, Event: ev})
}
func (s *AppServer) jobEvents(w http.ResponseWriter, r *http.Request, id string) {
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	if after < 0 {
		after = 0
	}
	s.mu.Lock()
	j := s.jobs[id]
	if j == nil {
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	after = min(after, len(j.Events))
	end := min(after+500, len(j.Events))
	events := append([]sequencedEvent{}, j.Events[after:end]...)
	out := map[string]any{"id": id, "events": events, "done": j.Done, "output": j.Output, "next": end, "more": end < len(j.Events), "limit": j.Limit, "inputIds": j.InputIDs}
	s.mu.Unlock()
	sendJSON(w, 200, out)
}
func (s *AppServer) resultRecord(id, index string) (Record, error) {
	n, e := strconv.Atoi(index)
	if e != nil {
		return Record{}, fmt.Errorf("结果索引无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	if j == nil || n < 0 || n >= len(j.Records) {
		return Record{}, fmt.Errorf("结果不存在")
	}
	rec := j.Records[n]
	if rec.Output == "" {
		return Record{}, fmt.Errorf("此结果没有导出文件")
	}
	return rec, nil
}
func (s *AppServer) result(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	rec, e := s.resultRecord(parts[0], parts[1])
	if e != nil {
		apiError(w, 404, e)
		return
	}
	serveLocalFile(w, r, rec.Output, r.URL.Query().Get("download") == "1")
}
func serveLocalFile(w http.ResponseWriter, r *http.Request, path string, download bool) {
	f, e := os.Open(path)
	if e != nil {
		apiError(w, 404, fmt.Errorf("文件已被移动或删除"))
		return
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	if download {
		w.Header().Set("Content-Disposition", "attachment")
	}
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}
func (s *AppServer) openResult(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Job    string `json:"job"`
		Index  string `json:"index"`
		Folder bool   `json:"folder"`
	}
	if e := decodeBody(w, r, &q, 4096); e != nil {
		apiError(w, 400, e)
		return
	}
	path := ""
	if q.Folder {
		s.mu.Lock()
		if j := s.jobs[q.Job]; j != nil {
			path = j.Output
		}
		s.mu.Unlock()
	} else {
		rec, e := s.resultRecord(q.Job, q.Index)
		if e != nil {
			apiError(w, 400, e)
			return
		}
		path = rec.Output
	}
	if path == "" {
		apiError(w, 400, fmt.Errorf("尚未生成结果目录"))
		return
	}
	if _, e := os.Stat(path); e != nil {
		apiError(w, 404, fmt.Errorf("结果已移动或删除"))
		return
	}
	if e := openLocalPath(path); e != nil {
		apiError(w, 400, e)
		return
	}
	sendJSON(w, 200, map[string]bool{"ok": true})
}
func (s *AppServer) lease(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.leases++
	s.lastLease = time.Now()
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.leases--; s.lastLease = time.Now(); s.mu.Unlock() }()
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, e := io.WriteString(w, ": alive\n\n"); e != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// serveApplication has no public listening address. A fresh cryptographic token
// and ephemeral loopback port isolate every launch from other websites/sessions.
func serveApplication(ctx context.Context, port int, readyFile, dataDir string, openWindow bool) error {
	listener, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if e != nil {
		return e
	}
	s, e := newAppServer(dataDir, desktopRenderer)
	if e != nil {
		listener.Close()
		return e
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.stop = cancel
	s.host = listener.Addr().String()
	server := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16384}
	errch := make(chan error, 1)
	go func() { errch <- server.Serve(listener) }()
	url := "http://" + s.host + "/#token=" + s.token
	if readyFile != "" {
		b, _ := json.Marshal(map[string]string{"url": url, "token": s.token, "address": "http://" + s.host, "version": version})
		if e = os.WriteFile(readyFile, b, 0600); e != nil {
			server.Close()
			return e
		}
	}
	if openWindow {
		if e = launchAppWindow(url); e != nil {
			server.Close()
			return e
		}
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.mu.Lock()
					idle := s.leases == 0 && time.Since(s.lastLease) > 45*time.Second
					s.mu.Unlock()
					if idle {
						cancel()
						return
					}
				}
			}
		}()
	}
	select {
	case <-ctx.Done():
		_ = server.Close()
		return nil
	case e = <-errch:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	}
}
