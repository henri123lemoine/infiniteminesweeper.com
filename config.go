package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var (
	walFlushInterval time.Duration = 2 * time.Minute
	snapshotInterval time.Duration = 30 * time.Minute
)

func isDevMode() bool {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Error loading .env file: %v", err)
	}

	d := strings.ToLower(os.Getenv("DEV"))
	return d == "1" || d == "true" || d == "yes"
}

func init() {
	if isDevMode() {
		walFlushInterval = 10 * time.Second
		snapshotInterval = 2 * time.Minute
	}
}
