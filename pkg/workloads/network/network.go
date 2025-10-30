package network

import (
	"net/http"

	"github.com/bpftrace/playground/pkg/workloads"
)

// web just executes a simple web request.
type web struct{}

func (*web) Name() string { return "web" }
func (*web) Execute() error {
	// Just fetch the contents of google.com using the stdlib http.
	resp, err := http.Get("https://www.google.com")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func init() {
	workloads.Register(&web{})
}
