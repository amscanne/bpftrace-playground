package download

import (
	"container/list"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

const urlFmt = "https://github.com/bpftrace/bpftrace/releases/%s/download/bpftrace"

// Manager handles downloading and caching of binaries.
type Manager struct {
	cacheDir string
	maxCache int
	mu       sync.Mutex
	cache    map[string]string
	lru      *list.List
	urlFmt   string
}

// NewManager creates a new Manager.
func NewManager(cacheDir string, maxCache int) (*Manager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}
	return &Manager{
		cacheDir: cacheDir,
		maxCache: maxCache,
		cache:    make(map[string]string),
		lru:      list.New(),
		urlFmt:   urlFmt,
	}, nil
}

// Get returns the path to the binary for the given version, downloading it if necessary.
func (m *Manager) Get(version string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if path, ok := m.cache[version]; ok {
		// Move to front of LRU.
		for e := m.lru.Front(); e != nil; e = e.Next() {
			if e.Value.(string) == version {
				m.lru.MoveToFront(e)
				break
			}
		}
		return path, nil
	}

	path := filepath.Join(m.cacheDir, version, "bpftrace")
	if _, err := os.Stat(path); err == nil {
		m.add(version, path)
		return path, nil
	}

	url := fmt.Sprintf(m.urlFmt, version)
	if err := m.download(url, path); err != nil {
		return "", err
	}

	m.add(version, path)
	return path, nil
}

func (m *Manager) add(version, path string) {
	if m.lru.Len() >= m.maxCache {
		// Evict oldest.
		e := m.lru.Back()
		if e != nil {
			oldVersion := e.Value.(string)
			m.lru.Remove(e)
			delete(m.cache, oldVersion)
			os.RemoveAll(filepath.Dir(m.cache[oldVersion]))
		}
	}
	m.lru.PushFront(version)
	m.cache[version] = path
}

func (m *Manager) download(url, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}

	return os.Chmod(path, 0755)
}
