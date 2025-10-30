package download

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v51/github"
	"golang.org/x/oauth2"
)

// Manager handles finding workflow runs and downloading artifacts from GitHub Actions.
type Manager struct {
	cacheDir   string
	maxCache   int
	mu         sync.Mutex
	cache      map[string]string
	lru        []string
	owner      string
	repo       string
	workflow   string
	client     *github.Client
	httpClient *http.Client

	// Version resolution cache: maps branch name to resolved run info.
	versionCache    map[string]*versionCacheEntry
	versionCacheTTL time.Duration
}

type versionCacheEntry struct {
	runID     int64
	sha       string
	timestamp time.Time
}

// NewManager creates a new Manager. If token is empty, requests are unauthenticated.
func NewManager(cacheDir string, maxCache int, owner, repo, workflow, token string) (*Manager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	var httpClient *http.Client
	tc := context.Background()
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(tc, ts)
	} else {
		httpClient = http.DefaultClient
	}

	client := github.NewClient(httpClient)

	return &Manager{
		cacheDir:        cacheDir,
		maxCache:        maxCache,
		cache:           make(map[string]string),
		lru:             make([]string, 0, maxCache),
		owner:           owner,
		repo:            repo,
		workflow:        workflow,
		client:          client,
		httpClient:      httpClient,
		versionCache:    make(map[string]*versionCacheEntry),
		versionCacheTTL: 30 * time.Minute,
	}, nil
}

func (m *Manager) findArtifactInRun(ctx context.Context, runID int64, artifactName string) (*github.Artifact, error) {
	opts := &github.ListOptions{PerPage: 100}
	for {
		ars, resp, artifactErr := m.client.Actions.ListWorkflowRunArtifacts(ctx, m.owner, m.repo, runID, opts)
		if artifactErr != nil {
			return nil, artifactErr
		}
		for _, a := range ars.Artifacts {
			if a.GetName() == artifactName {
				return a, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return nil, nil
}

func (m *Manager) downloadArtifact(ctx context.Context, artifact *github.Artifact) (*bytes.Buffer, error) {
	art, _, err := m.client.Actions.GetArtifact(ctx, m.owner, m.repo, artifact.GetID())
	if err != nil {
		return nil, err
	}
	url := art.GetArchiveDownloadURL()
	if url == "" {
		return nil, fmt.Errorf("artifact %d has no download URL", art.GetID())
	}

	// Download archive via httpClient (which includes auth if token provided).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download artifact: %s", resp.Status)
	}

	buf := new(bytes.Buffer)
	if _, copyErr := io.Copy(buf, resp.Body); copyErr != nil {
		return nil, copyErr
	}
	return buf, nil
}

func (m *Manager) extractSingleFile(buf *bytes.Buffer, targetDir string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return "", err
	}

	var singleFile *zip.File
	fileCount := 0
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		fileCount++
		if fileCount > 1 {
			return "", errors.New("multiple files in archive")
		}
		singleFile = f
	}

	if fileCount != 1 || singleFile == nil {
		return "", errors.New("no single file found")
	}

	rc, err := singleFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	outPath := filepath.Join(targetDir, filepath.Base(singleFile.Name))
	outF, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer outF.Close()

	//nolint:gosec // Artifacts from GitHub Actions are trusted
	if _, err := io.Copy(outF, rc); err != nil {
		return "", err
	}

	_ = os.Chmod(outPath, 0755)
	return outPath, nil
}

// GetArtifact finds the appropriate workflow run and downloads the named artifact.
// - artifactName: name of the artifact to download
// - version: branch name, tag, or commit SHA to match against runs
// Returns the path to the downloaded artifact (zip or extracted file when single-file artifact).
func (m *Manager) GetArtifact(ctx context.Context, artifactName, version string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Try to resolve from cache first.
	var resolvedSHA string

	if entry, ok := m.versionCache[version]; ok {
		if time.Since(entry.timestamp) < m.versionCacheTTL {
			resolvedSHA = entry.sha
			artifactKey := resolvedSHA + "|" + artifactName
			if p, ok := m.cache[artifactKey]; ok {
				return p, nil
			}
		}
	}

	// Find the run
	run, err := m.findRun(ctx, version)
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", errors.New("no matching workflow run found")
	}

	// Update the version cache with the resolved run info
	sha := run.GetHeadSHA()
	if sha == "" {
		return "", fmt.Errorf("unable to resolve SHA for workflow run %d", run.GetID())
	}

	m.versionCache[version] = &versionCacheEntry{
		runID:     run.GetID(),
		sha:       sha,
		timestamp: time.Now(),
	}
	resolvedSHA = sha

	// Check artifact cache again with resolved SHA
	artifactKey := resolvedSHA + "|" + artifactName
	if p, ok := m.cache[artifactKey]; ok {
		return p, nil
	}

	// Find the artifact
	artifact, err := m.findArtifactInRun(ctx, run.GetID(), artifactName)
	if err != nil {
		return "", err
	}
	if artifact == nil {
		return "", fmt.Errorf("artifact %q not found on run %d", artifactName, run.GetID())
	}

	// Download the artifact
	buf, err := m.downloadArtifact(ctx, artifact)
	if err != nil {
		return "", err
	}

	// Write to cache directory
	targetDir := filepath.Join(m.cacheDir, resolvedSHA)
	if mkdirErr := os.MkdirAll(targetDir, 0755); mkdirErr != nil {
		return "", mkdirErr
	}

	// Save as zip file
	zipPath := filepath.Join(targetDir, artifactName+".zip")
	if writeErr := os.WriteFile(zipPath, buf.Bytes(), 0600); writeErr != nil {
		return "", writeErr
	}

	// Try to extract single file
	if extractedPath, err := m.extractSingleFile(buf, targetDir); err == nil {
		m.addCache(resolvedSHA, artifactName, extractedPath)
		return extractedPath, nil
	}

	// Fallback: return zip path
	m.addCache(resolvedSHA, artifactName, zipPath)
	return zipPath, nil
}

func (m *Manager) addCache(version, artifactName, path string) {
	key := version + "|" + artifactName
	// simple LRU via slice
	if _, ok := m.cache[key]; ok {
		m.cache[key] = path
		return
	}
	if len(m.lru) >= m.maxCache {
		// evict oldest
		oldKey := m.lru[0]
		m.lru = m.lru[1:]
		if p, ok := m.cache[oldKey]; ok {
			_ = os.RemoveAll(filepath.Dir(p))
			delete(m.cache, oldKey)
		}
	}
	m.lru = append(m.lru, key)
	m.cache[key] = path
}

// findRun locates a workflow run matching the given version.
// - If version is empty, returns the latest successful run for the workflow.
// - If version matches a branch name, returns the latest successful run for that branch.
// - If version is a commit SHA (or prefix), returns a run with a matching head SHA.
func (m *Manager) findRun(ctx context.Context, version string) (*github.WorkflowRun, error) {
	opts := &github.ListWorkflowRunsOptions{ListOptions: github.ListOptions{PerPage: 30}}

	// If version looks like a branch name, filter by branch.
	if version != "" && !looksLikeCommitSHA(version) {
		opts.Branch = version
	}

	// Iterate pages until match is found or exhausted.
	for {
		runs, resp, err := m.client.Actions.ListWorkflowRunsByFileName(ctx, m.owner, m.repo, m.workflow, opts)
		if err != nil {
			return nil, err
		}
		for _, run := range runs.WorkflowRuns {
			// Only consider completed runs
			if run.GetStatus() != "completed" {
				continue
			}

			// If no version specified or we're filtering by branch, take first successful run.
			if version == "" || opts.Branch != "" {
				if run.GetConclusion() == "success" {
					return run, nil
				}
				continue
			}

			// version provided and looks like SHA: match by head SHA prefix.
			if hs := run.GetHeadSHA(); hs != "" {
				if strings.HasPrefix(hs, version) {
					return run, nil
				}
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return nil, nil
}

// looksLikeCommitSHA returns true if the string looks like a commit SHA (hex string).
func looksLikeCommitSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
