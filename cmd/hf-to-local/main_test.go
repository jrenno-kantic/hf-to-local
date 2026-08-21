package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// emptyCache returns a valid but empty HF cache directory, which lets the CLI
// exercise a full successful run without any fixture symlinks.
func emptyCache(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "hub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func exec(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut strings.Builder
	code = run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionFlag(t *testing.T) {
	code, stdout, _ := exec(t, "--version")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if strings.TrimSpace(stdout) != version {
		t.Fatalf("stdout = %q, want %q", stdout, version)
	}
}

// list previously had no --version, so the two flag sets could drift.
func TestListVersionFlag(t *testing.T) {
	code, stdout, _ := exec(t, "list", "--version")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if strings.TrimSpace(stdout) != version {
		t.Fatalf("stdout = %q, want %q", stdout, version)
	}
}

func TestMissingTargetIsAnError(t *testing.T) {
	code, _, stderr := exec(t, "--cache", emptyCache(t))
	if code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "--target is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestUnknownFlagExitsUsage(t *testing.T) {
	code, _, stderr := exec(t, "--nope")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "not defined") {
		t.Fatalf("stderr = %q, want a flag error", stderr)
	}
}

func TestUnsupportedRuntimeIsAnError(t *testing.T) {
	code, _, stderr := exec(t, "--target", t.TempDir(), "--cache", emptyCache(t), "--runtime", "bogus")
	if code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "unsupported runtime") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestMissingCacheDirIsAnError(t *testing.T) {
	code, _, stderr := exec(t, "--target", t.TempDir(), "--cache", filepath.Join(t.TempDir(), "absent"))
	if code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "read HF cache directory") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestSummaryLineShape(t *testing.T) {
	code, stdout, stderr := exec(t, "--target", t.TempDir(), "--cache", emptyCache(t))
	if code != exitOK {
		t.Fatalf("exit = %d (stderr %q)", code, stderr)
	}
	for _, want := range []string{"done:", "repo(s)", "link(s)", "broken source(s)"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
}

func TestPlanJSONIsValidAndImpliesDryRun(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	code, stdout, stderr := exec(t, "--target", target, "--cache", emptyCache(t), "--plan", "json")
	if code != exitOK {
		t.Fatalf("exit = %d (stderr %q)", code, stderr)
	}

	var summary struct {
		ReposSeen    int `json:"repos_seen"`
		LinksCreated int `json:"links_created"`
	}
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("plan is not valid JSON: %v (%q)", err, stdout)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("--plan json must not create the target directory")
	}
}

func TestListJSONInventory(t *testing.T) {
	code, stdout, stderr := exec(t, "list", "--cache", emptyCache(t), "--json")
	if code != exitOK {
		t.Fatalf("exit = %d (stderr %q)", code, stderr)
	}
	var inventory struct {
		CacheDir string `json:"cache_dir"`
	}
	if err := json.Unmarshal([]byte(stdout), &inventory); err != nil {
		t.Fatalf("inventory is not valid JSON: %v (%q)", err, stdout)
	}
	if inventory.CacheDir == "" {
		t.Fatal("inventory omitted cache_dir")
	}
}

func TestListTableHeader(t *testing.T) {
	code, stdout, _ := exec(t, "list", "--cache", emptyCache(t))
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "REPO") || !strings.Contains(stdout, "BROKEN") {
		t.Fatalf("stdout = %q, want the table header", stdout)
	}
}

func TestCanceledContextExitsInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr strings.Builder
	code := run(ctx, []string{"--target", t.TempDir(), "--cache", emptyCache(t)}, &stdout, &stderr)
	// An empty cache can finish before the cancellation is observed; either
	// outcome is valid, but a cancelled run must never report a plain error.
	if code != exitOK && code != exitInterrupted {
		t.Fatalf("exit = %d, want %d or %d (stderr %q)", code, exitOK, exitInterrupted, stderr.String())
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:                      "0 B",
		512:                    "512 B",
		1024:                   "1.0 KiB",
		1024 * 1024:            "1.0 MiB",
		3 * 1024 * 1024 * 1024: "3.0 GiB",
	}
	for size, want := range cases {
		if got := humanBytes(size); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", size, got, want)
		}
	}
}
