package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testApp(t *testing.T) *AppServer {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	s, err := newAppServer(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func requestJSON(t *testing.T, s *AppServer, method, path string, value any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, "http://localhost/api/"+path, bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer "+s.token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestPreferenceMarginRoundTrip(t *testing.T) {
	s := testApp(t)
	for _, margin := range []float64{0, 2, 5, 20} {
		p := Preferences{Theme: "light", Mode: "pdf-images", Output: t.TempDir(), Tools: map[string]PreferenceMode{"pdf-images": {Amount: "500", Unit: "KB", MarginMM: &margin}}}
		w := requestJSON(t, s, "PUT", "prefs", p)
		if w.Code != 200 {
			t.Fatalf("margin %v: %s", margin, w.Body.String())
		}
		b, err := os.ReadFile(s.prefsPath)
		if err != nil {
			t.Fatal(err)
		}
		var stored Preferences
		if err = json.Unmarshal(b, &stored); err != nil {
			t.Fatal(err)
		}
		if got := stored.Tools["pdf-images"].MarginMM; got == nil || *got != margin {
			t.Fatalf("margin %v was not preserved", margin)
		}
	}
	for _, margin := range []float64{-1, 21} {
		p := s.prefs
		p.Tools["pdf-images"] = PreferenceMode{Amount: "500", Unit: "KB", MarginMM: &margin}
		if w := requestJSON(t, s, "PUT", "prefs", p); w.Code != 400 {
			t.Fatalf("accepted invalid margin %v", margin)
		}
	}
}

func TestLegacyPreferencesWithoutMargin(t *testing.T) {
	var p Preferences
	if err := json.Unmarshal([]byte(`{"theme":"light","mode":"pdf-images","tools":{"pdf-images":{"amount":"2","unit":"MB"}}}`), &p); err != nil {
		t.Fatal(err)
	}
	p.Output = t.TempDir()
	if err := validatePrefs(p); err != nil {
		t.Fatal(err)
	}
	if p.Tools["pdf-images"].MarginMM != nil {
		t.Fatal("missing margin must remain distinct from explicit zero")
	}
}

func TestBuiltInExamplesRequireSession(t *testing.T) {
	s := testApp(t)
	r := httptest.NewRequest("GET", "http://localhost/api/example/crop-demo.pdf", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("example endpoint bypassed session authorization")
	}
	for _, name := range []string{"crop-demo.pdf", "text-demo.png"} {
		w = requestJSON(t, s, "GET", "example/"+name, nil)
		if w.Code != 200 || w.Body.Len() < 100 {
			t.Fatalf("missing built-in example %s", name)
		}
	}
	if w = requestJSON(t, s, "GET", "example/unknown.pdf", nil); w.Code != 404 {
		t.Fatal("unexpected sample name accepted")
	}
}

func TestAutoCropPreviewOwnershipAndMargin(t *testing.T) {
	s := testApp(t)
	im := image.NewRGBA(image.Rect(0, 0, 400, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			c := color.RGBA{255, 255, 255, 255}
			if x >= 100 && x < 300 && y >= 100 && y < 300 {
				c = color.RGBA{20, 20, 20, 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	path := filepath.Join(s.root, "preview.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, im)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	s.files["one"] = &FileItem{ID: "one", Kind: "pdf", Path: path}
	s.files["two"] = &FileItem{ID: "two", Kind: "pdf", Path: path}
	s.previewByKey["key"] = Preview{FileID: "one", Key: "key", Path: path, Width: 400, Height: 400, PageWidth: 384, PageHeight: 384}
	request := map[string]any{"id": "two", "previewKey": "key", "edit": ImageEdit{}, "marginMM": 0}
	if w := requestJSON(t, s, "POST", "autocrop", request); w.Code != 400 {
		t.Fatal("preview from another file was accepted")
	}
	request["id"] = "one"
	var rects []NormalizedRect
	for _, margin := range []float64{0, 5} {
		request["marginMM"] = margin
		w := requestJSON(t, s, "POST", "autocrop", request)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var result struct {
			Rect NormalizedRect `json:"rect"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		rects = append(rects, result.Rect)
	}
	if rects[1].Left >= rects[0].Left || rects[1].Right <= rects[0].Right {
		t.Fatalf("5 mm must retain more content: %+v", rects)
	}
}
