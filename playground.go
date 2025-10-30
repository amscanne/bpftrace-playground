package main

import (
	"flag"
	"log"
	"os"
	"strconv"

	"github.com/bpftrace/playground/pkg/download"
	"github.com/bpftrace/playground/pkg/service"
	"github.com/bpftrace/playground/pkg/workloads"
	_ "github.com/bpftrace/playground/pkg/workloads/basic"
	_ "github.com/bpftrace/playground/pkg/workloads/files"
	_ "github.com/bpftrace/playground/pkg/workloads/network"
)

var (
	port       = flag.String("port", getEnvOrDefault("PORT", "8088"), "Port to listen on")
	cacheDir   = flag.String("cache-dir", getEnvOrDefault("CACHE_DIR", "/tmp/cache"), "Cache directory")
	maxCache   = flag.Int("max-cache", getEnvIntOrDefault("MAX_CACHE", 5), "Maximum cache entries")
	maxTimeout = flag.Int("max-timeout", getEnvIntOrDefault("MAX_TIMEOUT", 30000), "Maximum timeout in milliseconds")
	owner      = flag.String("owner", getEnvOrDefault("GITHUB_OWNER", "bpftrace"), "GitHub repository owner")
	repo       = flag.String("repo", getEnvOrDefault("GITHUB_REPO", "bpftrace"), "GitHub repository name")
	workflow   = flag.String("workflow", getEnvOrDefault("GITHUB_WORKFLOW", "binary.yml"), "GitHub Actions workflow name")
	token      = flag.String("token", getEnvOrDefault("GITHUB_TOKEN", ""), "GitHub token for authentication")
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

	switch flag.NArg() {
	case 0:
		downloader, err := download.NewManager(*cacheDir, *maxCache, *owner, *repo, *workflow, *token)
		if err != nil {
			log.Fatalf("Downloader failed: %v", err)
		}
		if err := service.Main(*port, downloader, *maxTimeout); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	case 1:
		arg := flag.Args()[0]
		if err := workloads.Run(arg); err != nil {
			log.Fatalf("failed: %v", err)
		}
	default:
		flag.PrintDefaults()
		os.Exit(1)
	}
}
