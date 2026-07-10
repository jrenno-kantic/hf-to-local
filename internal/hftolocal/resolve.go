package hftolocal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// hfModelsAPI is the Hugging Face model metadata endpoint. Requesting an
// org-less (legacy canonical) name returns a 307 redirect to the canonical
// "org/name" route, which the default HTTP client follows transparently.
const hfModelsAPI = "https://huggingface.co/api/models/"

// resolveTimeout bounds a single canonical-name lookup so an unreachable
// network never stalls the list command.
const resolveTimeout = 5 * time.Second

var sharedHTTPClient = &http.Client{Timeout: resolveTimeout}

// nameResolver maps a bare repo id (no "/") to its canonical "org/name".
type nameResolver func(ctx context.Context, bareID string) (string, error)

// defaultNameResolver queries the live Hugging Face API.
func defaultNameResolver(ctx context.Context, bareID string) (string, error) {
	return httpResolveRepoID(ctx, sharedHTTPClient, hfModelsAPI, bareID)
}

func httpResolveRepoID(ctx context.Context, client *http.Client, baseURL, bareID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+url.PathEscape(bareID), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hf api status %d for %q", resp.StatusCode, bareID)
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.ID == "" {
		return "", fmt.Errorf("hf api returned empty id for %q", bareID)
	}
	return payload.ID, nil
}

// resolveRepoNames rewrites org-less repo ids to their canonical "org/name"
// form. Repos that already carry an org, and lookups that fail (offline,
// ambiguous, or genuinely org-less repos), keep their original id. Successful
// resolutions are memoized on disk so repeated runs stay offline-friendly.
func resolveRepoNames(repos []CachedRepo, resolve nameResolver, cachePath string, timeout time.Duration) {
	if resolve == nil {
		return
	}
	names := loadNameCache(cachePath)
	changed := false
	for i := range repos {
		id := repos[i].RepoID
		if strings.Contains(id, "/") {
			continue
		}
		if canonical, ok := names[id]; ok {
			if canonical != "" {
				repos[i].RepoID = canonical
			}
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		canonical, err := resolve(ctx, id)
		cancel()
		if err != nil || canonical == "" {
			continue // keep bare name; retry on a later run
		}
		names[id] = canonical
		changed = true
		repos[i].RepoID = canonical
	}
	if changed {
		saveNameCache(cachePath, names)
	}
}

// defaultNameCachePath returns the persistent lookup cache location, or "" when
// no user cache directory is available (persistence is then skipped).
func defaultNameCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "hf-to-local", "repo-names.json")
}

func loadNameCache(path string) map[string]string {
	names := map[string]string{}
	if path == "" {
		return names
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return names
	}
	var payload struct {
		Names map[string]string `json:"names"`
	}
	if json.Unmarshal(data, &payload) == nil && payload.Names != nil {
		names = payload.Names
	}
	return names
}

func saveNameCache(path string, names map[string]string) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(struct {
		Names map[string]string `json:"names"`
	}{Names: names}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}
