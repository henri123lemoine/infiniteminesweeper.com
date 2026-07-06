package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
)

// walFlushInterval bounds how much play a hard kill (OOM) can lose. Flushes
// are appends to local NVMe, so a short interval costs almost nothing.
// Graceful stops (deploys, Fly auto-stop) lose nothing regardless — the
// SIGINT handler flushes on the way out.
var (
	walFlushInterval time.Duration = 15 * time.Second
	snapshotInterval time.Duration = 30 * time.Minute
)

// Loads .env.shared, .env.development, or .env.production
// depending on the MODE environment variable.
// Either way, .env.shared is always loaded.
func mustLoadEnv() {
	env := os.Getenv("MODE") // development / production

	wd, _ := os.Getwd()
	if filepath.Base(wd) == "backend" {
		wd = filepath.Dir(wd) // go up one level
	}

	root := func(name string) string { return filepath.Join(wd, name) }

	files := []string{root(".env.shared")}
	switch env {
	case "development":
		files = append(files, root(".env.development"))
	case "production":
		files = append(files, root(".env.production"))
	}

	if err := godotenv.Load(files...); err != nil {
		log.Printf("env-loader: %v", err)
	}
}

func init() {
	if os.Getenv("MODE") == "development" {
		walFlushInterval = 10 * time.Second
		snapshotInterval = 2 * time.Minute
	}
}
