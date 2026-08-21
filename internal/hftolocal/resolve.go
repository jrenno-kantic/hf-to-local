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

// resolveBudget bounds the whole resolution pass. Without it a cache holding
// many legacy org-less repos multiplies resolveTimeout by the repo count.
const resolveBudget = 30 * time.Second

// resolveAttempts is the total number of tries per lookup, including the first.
const resolveAttempts = 2

// resolveBackoff is the pause before a retry of a transient failure.
var resolveBackoff = 250 * time.Millisecond

// UserAgent identifies the client to the Hugging Face API. Anonymous traffic is
// rate limited more aggressively when it is unattributed; main overrides this
// with the build version.
var UserAgent = "hf-to-local"

var sharedHTTPClient = &http.Client{Timeout: resolveTimeout}

// nameResolver maps a bare repo id (no "/") to its canonical "org/name".
type nameResolver func(ctx context.Context, bareID string) (string, error)

// defaultNameResolver queries the live Hugging Face API.
func defaultNameResolver(ctx context.Context, bareID string) (string, error) {
	return httpResolveRepoID(ctx, sharedHTTPClient, hfModelsAPI, bareID)
}

func httpResolveRepoID(ctx context.Context, client *http.Client, baseURL, bareID string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < resolveAttempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(resolveBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}
		id, retryable, err := httpResolveOnce(ctx, client, baseURL, bareID)
		if err == nil {
			return id, nil
		}
		lastErr = err
		if !retryable {
			break
		}
	}
	return "", lastErr
}

// httpResolveOnce performs a single lookup. retryable distinguishes transient
// server-side conditions from a definitive answer such as 404, which must not
// be retried.
func httpResolveOnce(ctx context.Context, client *http.Client, baseURL, bareID string) (id string, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+url.PathEscape(bareID), nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		transient := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return "", transient, fmt.Errorf("hf api status %d for %q", resp.StatusCode, bareID)
	}

	var payload struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", false, err
	}
	if payload.ID == "" {
		return "", false, fmt.Errorf("hf api returned empty id for %q", bareID)
	}
	return payload.ID, false, nil
}

// resolveRepoNames rewrites org-less repo ids to their canonical "org/name"
// form. Repos that already carry an org, and lookups that fail (offline,
// ambiguous, or genuinely org-less repos), keep their original id. Successful
// resolutions are memoized on disk so repeated runs stay offline-friendly.
func resolveRepoNames(ctx context.Context, repos []CachedRepo, resolve nameResolver, cachePath string, timeout, budget time.Duration) {
	if resolve == nil {
		return
	}
	names := loadNameCache(cachePath)
	changed := false

	budgetCtx, cancelBudget := context.WithTimeout(ctx, budget)
	defer cancelBudget()

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
		if budgetCtx.Err() != nil {
			break // out of time; keep remaining bare names and retry on a later run
		}
		callCtx, cancel := context.WithTimeout(budgetCtx, timeout)
		canonical, err := resolve(callCtx, id)
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

// saveNameCache writes the lookup cache atomically. A torn write from a crash or
// from two concurrent runs would otherwise leave unparseable JSON behind.
func saveNameCache(path string, names map[string]string) {
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(struct {
		Names map[string]string `json:"names"`
	}{Names: names}, "", "  ")
	if err != nil {
		return
	}

	temp, err := os.CreateTemp(dir, ".repo-names-*.json")
	if err != nil {
		return
	}
	tempName := temp.Name()
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		os.Remove(tempName)
		return
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempName)
		return
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		os.Remove(tempName)
		return
	}
	if err := os.Rename(tempName, path); err != nil {
		os.Remove(tempName)
	}
}
