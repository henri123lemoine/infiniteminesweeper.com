package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestStaticHandler(t *testing.T) http.Handler {
	t.Helper()
	fsys := fstest.MapFS{
		"index.html":             {Data: []byte("<!doctype html><title>home</title>" + strings.Repeat(" ", 512))},
		"assets/app-123.js":      {Data: []byte(strings.Repeat("console.log(1);", 100))},
		"robots.txt":             {Data: []byte("User-agent: *\nAllow: /\n")},
		"how-to-play/index.html": {Data: []byte("<!doctype html><title>guide</title>")},
		"favicon.ico":            {Data: []byte{0x89, 'P', 'N', 'G'}},
	}
	files, err := loadStaticFiles(fsys)
	if err != nil {
		t.Fatalf("loadStaticFiles: %v", err)
	}
	return newStaticHandler(files, http.FileServer(http.FS(fsys)))
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestStaticServesPrecompressedIndex(t *testing.T) {
	h := newTestStaticHandler(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")
	rec := serve(h, req)

	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("code=%d encoding=%q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	body, _ := io.ReadAll(zr)
	if !strings.Contains(string(body), "<title>home</title>") {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestStaticHashedAssetsAreImmutableAndRevalidate(t *testing.T) {
	h := newTestStaticHandler(t)
	rec := serve(h, httptest.NewRequest("GET", "/assets/app-123.js", nil))
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("served gzip without Accept-Encoding")
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("Cache-Control = %q", got)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	req := httptest.NewRequest("GET", "/assets/app-123.js", nil)
	req.Header.Set("If-None-Match", etag)
	if rec := serve(h, req); rec.Code != http.StatusNotModified {
		t.Fatalf("conditional GET code = %d, want 304", rec.Code)
	}
}

func TestStaticNestedIndexAndContentTypes(t *testing.T) {
	h := newTestStaticHandler(t)
	if rec := serve(h, httptest.NewRequest("GET", "/how-to-play/", nil)); rec.Code != 200 || !strings.Contains(rec.Body.String(), "guide") {
		t.Fatalf("how-to-play: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec := serve(h, httptest.NewRequest("GET", "/how-to-play", nil)); rec.Code != http.StatusMovedPermanently {
		t.Fatalf("how-to-play without slash: code=%d", rec.Code)
	}
	if got := serve(h, httptest.NewRequest("GET", "/robots.txt", nil)).Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("robots.txt Content-Type = %q", got)
	}
	if got := serve(h, httptest.NewRequest("GET", "/favicon.ico", nil)).Header().Get("Content-Type"); got != "image/x-icon" {
		t.Fatalf("favicon Content-Type = %q", got)
	}
	if rec := serve(h, httptest.NewRequest("GET", "/missing", nil)); rec.Code != http.StatusNotFound {
		t.Fatalf("missing: code=%d", rec.Code)
	}
}

func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"":                  false,
		"gzip":              true,
		"deflate, gzip;q=1": true,
		"gzip;q=0":          false,
		"br":                false,
	}
	for header, want := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", header)
		if got := acceptsGzip(req); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestCanonicalHostRedirectsWWW(t *testing.T) {
	h := canonicalHost(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest("GET", "/how-to-play/?ref=x", nil)
	req.Host = "www.infiniteminesweeper.com"
	rec := serve(h, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("code = %d, want 301", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://infiniteminesweeper.com/how-to-play/?ref=x" {
		t.Fatalf("Location = %q", got)
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.Host = "infiniteminesweeper.com"
	if rec := serve(h, req); rec.Code != http.StatusTeapot {
		t.Fatalf("apex host was redirected: code = %d", rec.Code)
	}
}
