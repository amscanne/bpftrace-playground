package network

import (
	"github.com/bpftrace/bpftrace-playground/pkg/workloads"
	"net/http"
)

// web just executes a simple web request.
type web struct{}

func (*web) Name() string { return "web" }
func (*web) Execute() error {
	// Just fetch the contents of google.com using the stdlib http.
	_, err := http.Get("https://www.google.com")
	return err
}

func init() {
	workloads.Register(&web{})
}
