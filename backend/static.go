package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// staticFile is an embedded asset with a precomputed validator and, for text
// types, a gzip variant: compressing per request would compete with the game
// loop for the single core.
type staticFile struct {
	data  []byte
	gz    []byte
	etag  string
	ctype string
}

var extraMimeTypes = map[string]string{
	".ico":         "image/x-icon",
	".txt":         "text/plain; charset=utf-8",
	".webmanifest": "application/manifest+json",
}

var compressibleExts = map[string]bool{
	".css": true, ".html": true, ".js": true, ".json": true, ".svg": true,
	".txt": true, ".webmanifest": true, ".xml": true,
}

func newStaticFile(name string, data []byte) (*staticFile, error) {
	ext := path.Ext(name)
	ctype := mime.TypeByExtension(ext)
	if t, ok := extraMimeTypes[ext]; ok {
		ctype = t
	}
	sum := sha256.Sum256(data)
	f := &staticFile{data: data, etag: hex.EncodeToString(sum[:8]), ctype: ctype}
	if compressibleExts[ext] {
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return nil, err
		}
		if _, err := zw.Write(data); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		if buf.Len() < len(data) {
			f.gz = buf.Bytes()
		}
	}
	return f, nil
}

func loadStaticFiles(fsys fs.FS) (map[string]*staticFile, error) {
	files := make(map[string]*staticFile)
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		f, err := newStaticFile(name, data)
		if err != nil {
			return err
		}
		files[name] = f
		return nil
	})
	return files, err
}

func cacheControlFor(name string) string {
	switch {
	case strings.HasPrefix(name, "assets/"):
		return "public, max-age=31536000, immutable"
	case strings.HasSuffix(name, ".html"):
		return "no-cache"
	default:
		return "public, max-age=86400"
	}
}

// newStaticHandler serves embedded files with caching and precompressed gzip.
// Paths it doesn't know (directory redirects, 404s) fall through to fallback.
func newStaticHandler(files map[string]*staticFile, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		switch {
		case strings.HasPrefix(name, "@"):
			name = "index.html" // /@x,y location links open the game there
		case name == "" || strings.HasSuffix(name, "/"):
			name += "index.html"
		}
		f, ok := files[name]
		if !ok {
			fallback.ServeHTTP(w, r)
			return
		}
		serveStaticFile(w, r, name, f, cacheControlFor(name))
	})
}

func serveStaticFile(w http.ResponseWriter, r *http.Request, name string, f *staticFile, cacheControl string) {
	h := w.Header()
	h.Set("Cache-Control", cacheControl)
	if f.ctype != "" {
		h.Set("Content-Type", f.ctype)
	}
	body, etag := f.data, f.etag
	if f.gz != nil {
		h.Add("Vary", "Accept-Encoding")
		if acceptsGzip(r) {
			body, etag = f.gz, f.etag+"-gz"
			h.Set("Content-Encoding", "gzip")
		}
	}
	h.Set("ETag", `"`+etag+`"`)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(enc), "gzip") {
			return strings.ReplaceAll(params, " ", "") != "q=0"
		}
	}
	return false
}

// canonicalHost 301s www.<host> to <host> so search engines index one origin.
func canonicalHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if host, ok := strings.CutPrefix(r.Host, "www."); ok {
			http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}
