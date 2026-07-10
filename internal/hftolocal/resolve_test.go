package hftolocal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveRepoNames(t *testing.T) {
	calls := map[string]int{}
	resolve := func(_ context.Context, bare string) (string, error) {
		calls[bare]++
		if bare == "camembert-base" {
			return "almanach/camembert-base", nil
		}
		return "", errors.New("not found")
	}

	repos := []CachedRepo{
		{RepoID: "camembert-base"},
		{RepoID: "org/already-scoped"},
		{RepoID: "unknown-model"},
	}
	cachePath := filepath.Join(t.TempDir(), "names.json")
	resolveRepoNames(repos, resolve, cachePath, time.Second)

	if repos[0].RepoID != "almanach/camembert-base" {
		t.Fatalf("bare name not resolved: got %q", repos[0].RepoID)
	}
	if repos[1].RepoID != "org/already-scoped" {
		t.Fatalf("scoped name should be untouched: got %q", repos[1].RepoID)
	}
	if repos[2].RepoID != "unknown-model" {
		t.Fatalf("unresolved name should stay bare: got %q", repos[2].RepoID)
	}
	if calls["org/already-scoped"] != 0 {
		t.Fatalf("scoped name should not trigger a lookup")
	}

	// Second run: the resolved name is served from the on-disk cache, while the
	// failed lookup is retried (no negative caching).
	for k := range calls {
		delete(calls, k)
	}
	repos2 := []CachedRepo{{RepoID: "camembert-base"}, {RepoID: "unknown-model"}}
	resolveRepoNames(repos2, resolve, cachePath, time.Second)

	if repos2[0].RepoID != "almanach/camembert-base" {
		t.Fatalf("cached name not applied: got %q", repos2[0].RepoID)
	}
	if calls["camembert-base"] != 0 {
		t.Fatalf("resolved name should come from cache, got %d lookups", calls["camembert-base"])
	}
	if calls["unknown-model"] != 1 {
		t.Fatalf("failed lookup should be retried, got %d lookups", calls["unknown-model"])
	}
}

func TestResolveRepoNamesNilResolver(t *testing.T) {
	repos := []CachedRepo{{RepoID: "camembert-base"}}
	resolveRepoNames(repos, nil, "", time.Second)
	if repos[0].RepoID != "camembert-base" {
		t.Fatalf("nil resolver must leave repos untouched: got %q", repos[0].RepoID)
	}
}

func TestHTTPResolveRepoIDFollowsRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/camembert-base", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/models/almanach/camembert-base", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/api/models/almanach/camembert-base", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"almanach/camembert-base","author":"almanach"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := httpResolveRepoID(context.Background(), srv.Client(), srv.URL+"/api/models/", "camembert-base")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if got != "almanach/camembert-base" {
		t.Fatalf("unexpected canonical id: %q", got)
	}
}

func TestHTTPResolveRepoIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := httpResolveRepoID(context.Background(), srv.Client(), srv.URL+"/api/models/", "nope"); err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}

func TestNameCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.json")
	saveNameCache(path, map[string]string{"camembert-base": "almanach/camembert-base"})

	got := loadNameCache(path)
	if got["camembert-base"] != "almanach/camembert-base" {
		t.Fatalf("round-trip mismatch: %v", got)
	}

	// Missing file yields an empty, writable map rather than nil.
	empty := loadNameCache(filepath.Join(t.TempDir(), "absent.json"))
	if empty == nil {
		t.Fatal("expected non-nil map for missing cache file")
	}
	empty["x"] = "y"
}
