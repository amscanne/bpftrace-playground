package main

import (
	"flag"
	"log"
	"os"
	"strconv"

	"github.com/amscanne/bpftrace-playground/pkg/service"
)

var (
	port       = flag.String("port", getEnvOrDefault("PORT", "8088"), "Port to listen on")
	cacheDir   = flag.String("cache-dir", getEnvOrDefault("CACHE_DIR", "/tmp/cache"), "Cache directory")
	maxCache   = flag.Int("max-cache", getEnvIntOrDefault("MAX_CACHE", 5), "Maximum cache entries")
	maxTimeout = flag.Int("max-timeout", getEnvIntOrDefault("MAX_TIMEOUT", 30000), "Maximum timeout in milliseconds")
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func main() {
	flag.Parse()

	if err := service.Main(*port, *cacheDir, *maxCache, *maxTimeout); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
