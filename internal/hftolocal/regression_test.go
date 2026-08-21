package hftolocal

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A dangling snapshot->blob symlink is routine in a partially downloaded cache
// and must not abort the run.
func TestBrokenSourceIsSkippedNotFatal(t *testing.T) {
	repoID := "org/Test-GGUF"
	cache := makeHFCache(t, repoID)
	addSafetensorsAndBrokenLink(t, cache, repoID)
	target := t.TempDir()

	var stderr strings.Builder
	summary, err := Run(context.Background(), Options{
		CacheDir: cache, TargetDir: target, Runtime: RuntimeMirror,
		LinkMode: LinkSymlink, IncludeMetadata: true,
		Stdout: io.Discard, Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("run aborted on broken source: %v", err)
	}
	if summary.BrokenSources != 1 {
		t.Fatalf("BrokenSources = %d, want 1", summary.BrokenSources)
	}
	if summary.LinksCreated < 2 {
		t.Fatalf("LinksCreated = %d, want the healthy files still linked", summary.LinksCreated)
	}
	if !strings.Contains(stderr.String(), "broken source") {
		t.Fatalf("stderr = %q, want a broken-source warning", stderr.String())
	}
}

// Cache folder names decode "--" to "/", so a crafted folder can yield a repo id
// with ".." segments. Nothing may be written outside --target.
func TestCraftedRepoIDCannotEscapeTarget(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	original := filepath.Join(cache, RepoIDToCacheName("org/Test-GGUF"))
	crafted := filepath.Join(cache, "models--..--..--pwned")
	if err := os.Rename(original, crafted); err != nil {
		t.Fatal(err)
	}

	base := t.TempDir()
	target := filepath.Join(base, "sandbox", "target")
	if _, err := Run(context.Background(), Options{
		CacheDir: cache, TargetDir: target, Runtime: RuntimeAll,
		LinkMode: LinkSymlink, IncludeMetadata: true, Stdout: io.Discard,
	}); err != nil {
		t.Fatal(err)
	}

	for _, escape := range []string{
		filepath.Join(base, "sandbox", "pwned"),
		filepath.Join(base, "pwned"),
	} {
		if _, err := os.Stat(escape); err == nil {
			t.Fatalf("wrote outside --target at %s", escape)
		}
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("nothing written inside --target: %v", err)
	}
}

func TestSafeSegmentRejectsDotSegments(t *testing.T) {
	for _, input := range []string{".", "..", "...", "./", "../"} {
		if got := safeSegment(input); got == "." || got == ".." || strings.Trim(got, ".") == "" {
			t.Fatalf("safeSegment(%q) = %q, want a non-traversing segment", input, got)
		}
	}
	// Legitimate dotted labels must survive.
	if got := safeSegment("v0.2.0"); got != "v0.2.0" {
		t.Fatalf("safeSegment(\"v0.2.0\") = %q, want it preserved", got)
	}
}

func TestSafeRelPathSanitizesEveryElement(t *testing.T) {
	got := safeRelPath("../pwned")
	if strings.Contains(got, "..") {
		t.Fatalf("safeRelPath(\"../pwned\") = %q, want no traversal", got)
	}
	if got := safeRelPath("a/b"); got != filepath.Join("a", "b") {
		t.Fatalf("safeRelPath(\"a/b\") = %q, want nesting preserved", got)
	}
}

// --repo must accept either the legacy cached id or the canonical one that
// --resolve-names produces.
func TestFilterReposMatchesCachedAndCanonicalIDs(t *testing.T) {
	repos := []CachedRepo{{
		RepoID:   "almanach/camembert-base", // already resolved
		CacheDir: filepath.Join("/cache", "models--camembert-base"),
	}}

	for _, requested := range []string{"almanach/camembert-base", "camembert-base"} {
		got, err := filterRepos(repos, []string{requested}, true)
		if err != nil {
			t.Fatalf("filterRepos(%q) error: %v", requested, err)
		}
		if len(got) != 1 {
			t.Fatalf("filterRepos(%q) = %d repos, want 1", requested, len(got))
		}
	}
}

func TestFilterReposMissingRepoHintsAtResolveNames(t *testing.T) {
	repos := []CachedRepo{{RepoID: "camembert-base", CacheDir: filepath.Join("/cache", "models--camembert-base")}}

	_, err := filterRepos(repos, []string{"almanach/camembert-base"}, false)
	if err == nil {
		t.Fatal("expected an error for an unmatched repo")
	}
	if !strings.Contains(err.Error(), "--resolve-names") {
		t.Fatalf("error = %q, want a --resolve-names hint", err)
	}

	if _, err := filterRepos(repos, []string{"almanach/camembert-base"}, true); err == nil {
		t.Fatal("expected an error for an unmatched repo")
	} else if strings.Contains(err.Error(), "--resolve-names") {
		t.Fatalf("error = %q, should not suggest a flag already in use", err)
	}
}

// Re-running against an already-populated target must still offer its Ollama
// models, so `--create` works after a plain generate run.
func TestOllamaCreatesSurviveRerun(t *testing.T) {
	repoID := "org/Test-GGUF"
	cache := makeHFCache(t, repoID)
	target := t.TempDir()
	opts := Options{
		CacheDir: cache, TargetDir: target, Runtime: RuntimeOllama,
		LinkMode: LinkSymlink, IncludeMetadata: true, Stdout: io.Discard,
	}

	first, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.OllamaModels != 1 {
		t.Fatalf("first run OllamaModels = %d, want 1", first.OllamaModels)
	}

	second, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.OllamaModels != first.OllamaModels {
		t.Fatalf("second run OllamaModels = %d, want %d", second.OllamaModels, first.OllamaModels)
	}
	if second.FilesWritten != 0 {
		t.Fatalf("second run FilesWritten = %d, want 0 under --on-conflict skip", second.FilesWritten)
	}
	if second.FilesSkipped == 0 {
		t.Fatal("second run should report skipped destinations")
	}
}

func TestCanceledContextStopsRun(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Run(ctx, Options{
		CacheDir: cache, TargetDir: target, Runtime: RuntimeAll,
		LinkMode: LinkSymlink, IncludeMetadata: true, Stdout: io.Discard,
	}); err == nil {
		t.Fatal("expected a cancellation error")
	} else if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestVerboseWritesProgressToStderr(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	var stderr strings.Builder

	if _, err := Run(context.Background(), Options{
		CacheDir: cache, TargetDir: t.TempDir(), Runtime: RuntimeMirror,
		LinkMode: LinkSymlink, IncludeMetadata: true, Verbose: true,
		Stdout: io.Discard, Stderr: &stderr,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "org/Test-GGUF") {
		t.Fatalf("stderr = %q, want per-repo progress", stderr.String())
	}
}

// Copy mode must never leave a partial file that a later --on-conflict skip run
// would mistake for a complete one.
func TestCopyModeLeavesNoPartialFiles(t *testing.T) {
	repoID := "org/Test-GGUF"
	cache := makeHFCache(t, repoID)
	target := t.TempDir()

	if _, err := Run(context.Background(), Options{
		CacheDir: cache, TargetDir: target, Runtime: RuntimeMirror,
		LinkMode: LinkCopy, IncludeMetadata: true, Stdout: io.Discard,
	}); err != nil {
		t.Fatal(err)
	}

	err := filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".part") {
			return fmt.Errorf("temporary copy left behind: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
