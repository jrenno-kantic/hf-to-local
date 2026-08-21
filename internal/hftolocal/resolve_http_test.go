package hftolocal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveSendsUserAgent(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("User-Agent")
		json.NewEncoder(w).Encode(map[string]string{"id": "almanach/camembert-base"})
	}))
	defer server.Close()

	previous := UserAgent
	UserAgent = "hf-to-local/test"
	defer func() { UserAgent = previous }()

	if _, err := httpResolveRepoID(context.Background(), server.Client(), server.URL+"/", "camembert-base"); err != nil {
		t.Fatal(err)
	}
	if seen != "hf-to-local/test" {
		t.Fatalf("User-Agent = %q, want hf-to-local/test", seen)
	}
}

func TestResolveRetriesTransientFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id": "almanach/camembert-base"})
	}))
	defer server.Close()

	previous := resolveBackoff
	resolveBackoff = time.Millisecond
	defer func() { resolveBackoff = previous }()

	id, err := httpResolveRepoID(context.Background(), server.Client(), server.URL+"/", "camembert-base")
	if err != nil {
		t.Fatal(err)
	}
	if id != "almanach/camembert-base" {
		t.Fatalf("id = %q", id)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2 (one retry)", got)
	}
}

func TestResolveDoesNotRetryNotFound(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := httpResolveRepoID(context.Background(), server.Client(), server.URL+"/", "nope"); err == nil {
		t.Fatal("expected an error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 (404 is definitive)", got)
	}
}

// The overall budget must stop a pass that would otherwise multiply the
// per-call timeout by the number of org-less repos.
func TestResolveRespectsTotalBudget(t *testing.T) {
	var calls atomic.Int32
	slow := func(ctx context.Context, bareID string) (string, error) {
		calls.Add(1)
		<-ctx.Done()
		return "", ctx.Err()
	}
	repos := []CachedRepo{
		{RepoID: "one"}, {RepoID: "two"}, {RepoID: "three"}, {RepoID: "four"},
	}

	start := time.Now()
	resolveRepoNames(context.Background(), repos, slow, "", 20*time.Millisecond, 50*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 400*time.Millisecond {
		t.Fatalf("elapsed = %v, want the budget to cut the pass short", elapsed)
	}
	if got := calls.Load(); int(got) >= len(repos) {
		t.Fatalf("calls = %d, want fewer than %d attempts once the budget expires", got, len(repos))
	}
	for _, repo := range repos {
		if repo.RepoID == "" {
			t.Fatal("unresolved repos must keep their bare id")
		}
	}
}

func TestSaveNameCacheIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo-names.json")

	saveNameCache(path, map[string]string{"camembert-base": "almanach/camembert-base"})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "repo-names.json" {
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("directory = %v, want only the final file with no temp left behind", names)
	}
	if got := loadNameCache(path)["camembert-base"]; got != "almanach/camembert-base" {
		t.Fatalf("round trip = %q", got)
	}
}
