package basic

import (
	"time"

	"github.com/bpftrace/playground/pkg/workloads"
)

// sleeper just sleeps for a short period of time.
type sleeper struct{}

func (*sleeper) Name() string { return "sleep" }
func (*sleeper) Execute() error {
	// Sleep for 100 milliseconds.
	time.Sleep(100 * time.Millisecond)
	return nil
}

func init() {
	workloads.Register(&sleeper{})
}
