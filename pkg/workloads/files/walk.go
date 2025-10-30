package files

import (
	"os"
	"path/filepath"

	"github.com/bpftrace/bpftrace-playground/pkg/workloads"
)

// walker walks the full directory tree, and reads the contents
// of all the files that are present in /etc.
type walker struct{}

func (*walker) Name() string { return "files" }
func (*walker) Execute() error {
	// Using the standard library, walk /etc and read all files.
	return filepath.Walk("/etc", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if _, err := os.ReadFile(path); err != nil {
			return err
		}
		return nil
	})
}

func init() {
	workloads.Register(&walker{})
}
