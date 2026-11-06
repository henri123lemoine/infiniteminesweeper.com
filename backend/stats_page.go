package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// The page is rebuilt at most every statsPageTTL: it walks every chunk and
// encodes a PNG, which shouldn't run per request on the game's single core.
const (
	statsPageTTL      = 5 * time.Minute
	statsTopPlayers   = 100
	statsMapMaxSide   = 1600
	statsCacheControl = "public, max-age=300"
)

//go:embed stats.html
var statsTemplateSrc string

var statsTemplate = template.Must(template.New("stats").Funcs(template.FuncMap{
	"commas": formatThousands,
	"inc":    func(i int) int { return i + 1 },
}).Parse(statsTemplateSrc))

var overviewColorPalette = func() color.Palette {
	p := make(color.Palette, len(overviewPaletteRGB))
	for i, c := range overviewPaletteRGB {
		p[i] = color.RGBA{c[0], c[1], c[2], 0xff}
	}
	return p
}()

type statsPageData struct {
	Players       int
	Online        int
	Chunks        int
	CellsRevealed uint64
	Flags         uint64
	Explosions    uint64
	FarthestCells int64
	SpanX, SpanY  int64
	Top           []leaderboardEntry
	MapWidth      int
	MapHeight     int
	UpdatedAt     time.Time
}

type statsPageCache struct {
	mu      sync.Mutex
	builtAt time.Time
	html    *staticFile
	png     *staticFile
}

func (s *Server) handleStatsPage(w http.ResponseWriter, r *http.Request) {
	html, worldPNG, err := s.statsPageFiles()
	if err != nil {
		log.Printf("[stats] build failed: %v", err)
		http.Error(w, "stats unavailable", http.StatusInternalServerError)
		return
	}
	switch r.URL.Path {
	case "/stats/":
		serveStaticFile(w, r, "index.html", html, statsCacheControl)
	case "/stats/world.png":
		if worldPNG == nil {
			http.NotFound(w, r)
			return
		}
		serveStaticFile(w, r, "world.png", worldPNG, statsCacheControl)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) statsPageFiles() (*staticFile, *staticFile, error) {
	c := &s.statsPage
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.html != nil && time.Since(c.builtAt) < statsPageTTL {
		return c.html, c.png, nil
	}

	data, worldMap := s.snapshotStats()
	var pngFile *staticFile
	if worldMap != nil {
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, worldMap); err != nil {
			return nil, nil, err
		}
		f, err := newStaticFile("world.png", buf.Bytes())
		if err != nil {
			return nil, nil, err
		}
		pngFile = f
		data.MapWidth, data.MapHeight = worldMap.Rect.Dx(), worldMap.Rect.Dy()
	}

	var buf bytes.Buffer
	if err := statsTemplate.Execute(&buf, data); err != nil {
		return nil, nil, err
	}
	htmlFile, err := newStaticFile("index.html", buf.Bytes())
	if err != nil {
		return nil, nil, err
	}
	c.builtAt, c.html, c.png = time.Now(), htmlFile, pngFile
	return c.html, c.png, nil
}

func (s *Server) snapshotStats() (statsPageData, *image.Paletted) {
	s.playersMu.RLock()
	online := len(s.players)
	s.playersMu.RUnlock()

	d := statsPageData{Online: online, UpdatedAt: time.Now().UTC()}
	var minX, minY, maxX, maxY int64
	var farthest float64

	s.stateMu.RLock()
	d.Players = len(s.scores)
	d.Chunks = len(s.chunks)
	d.CellsRevealed = s.totalRevealed
	for _, f := range s.flags {
		d.Flags += uint64(f.len())
	}
	for _, e := range s.explosions {
		d.Explosions += uint64(len(e))
	}
	first := true
	for cid := range s.chunks {
		if first {
			minX, maxX, minY, maxY, first = cid.X, cid.X, cid.Y, cid.Y, false
		}
		minX, maxX = min(minX, cid.X), max(maxX, cid.X)
		minY, maxY = min(minY, cid.Y), max(maxY, cid.Y)
		farthest = max(farthest, math.Hypot(float64(cid.X), float64(cid.Y)))
	}
	worldMap := s.statsWorldMapLocked()
	board := s.collectLeaderboardLocked()
	s.stateMu.RUnlock()

	if !first {
		d.SpanX, d.SpanY = (maxX-minX+1)*ChunkSize, (maxY-minY+1)*ChunkSize
		d.FarthestCells = int64(math.Round(farthest * ChunkSize))
	}
	board = slices.DeleteFunc(board, func(e leaderboardEntry) bool { return strings.TrimSpace(e.Name) == "" })
	sortLeaderboard(board)
	d.Top = board[:min(len(board), statsTopPlayers)]
	return d, worldMap
}

// statsWorldMapLocked copies the most detailed global overview image that
// fits statsMapMaxSide, falling back to the coarsest one.
// Caller must hold s.stateMu.
func (s *Server) statsWorldMapLocked() *image.Paletted {
	var chosen *overviewImage
	var chosenLOD int
	for _, lod := range overviewGlobalLODs {
		img := s.overviewImages[lod]
		if img == nil {
			continue
		}
		fits := img.WidthChunks*int(lod) <= statsMapMaxSide && img.HeightChunks*int(lod) <= statsMapMaxSide
		if chosen == nil || fits {
			chosen, chosenLOD = img, int(lod)
		}
	}
	if chosen == nil {
		return nil
	}
	w, h := chosen.WidthChunks*chosenLOD, chosen.HeightChunks*chosenLOD
	if len(chosen.Pixels) != w*h {
		return nil
	}
	return &image.Paletted{
		Pix:     slices.Clone(chosen.Pixels),
		Stride:  w,
		Rect:    image.Rect(0, 0, w, h),
		Palette: overviewColorPalette,
	}
}

func formatThousands(v any) string {
	s := fmt.Sprint(v)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return sign + b.String()
}
