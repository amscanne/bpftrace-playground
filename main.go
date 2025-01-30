package main

import (
	"flag"
	"log"

	"github.com/amscanne/bpftrace-playground/pkg/service"
)

func main() {
	port := flag.String("port", "8080", "port to listen on")
	cacheDir := flag.String("cache-dir", "/tmp/bpftrace-binaries", "directory to cache bpftrace binaries")
	maxCache := flag.Int("max-cache-entries", 10, "maximum number of bpftrace binaries to cache")
	maxTimeout := flag.Int("max-timeout", 10000, "maximum timeout for bpftrace execution in milliseconds")
	flag.Parse()

	if err := service.Main(*port, *cacheDir, *maxCache, *maxTimeout); err != nil {
		log.Fatal(err)
	}
}
