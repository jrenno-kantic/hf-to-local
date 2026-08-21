package hftolocal

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requireSymlinks skips a test when the platform will not let this process
// create symlinks. Windows needs Developer Mode or elevation, and every fixture
// here models the HF cache's snapshot->blob symlink layout.
func requireSymlinks(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable on %s: %v", runtime.GOOS, err)
	}
}

func makeHFCache(t *testing.T, repoID string) string {
	t.Helper()
	requireSymlinks(t)
	cache := filepath.Join(t.TempDir(), "cache")
	repoCache := filepath.Join(cache, RepoIDToCacheName(repoID))
	commit := "abc123"
	blobs := filepath.Join(repoCache, "blobs")
	snapshot := filepath.Join(repoCache, "snapshots", commit)
	refs := filepath.Join(repoCache, "refs")

	for _, dir := range []string{blobs, snapshot, refs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(refs, "main"), []byte(commit), 0o644); err != nil {
		t.Fatal(err)
	}

	ggufBlob := filepath.Join(blobs, "ggufblob")
	if err := os.WriteFile(ggufBlob, []byte("fake gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "blobs", "ggufblob"), filepath.Join(snapshot, "test.Q4_K_M.gguf")); err != nil {
		t.Fatal(err)
	}

	readmeBlob := filepath.Join(blobs, "readmeblob")
	if err := os.WriteFile(readmeBlob, []byte("readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "blobs", "readmeblob"), filepath.Join(snapshot, "README.md")); err != nil {
		t.Fatal(err)
	}

	return cache
}

func addSafetensorsAndBrokenLink(t *testing.T, cache, repoID string) {
	t.Helper()
	repoCache := filepath.Join(cache, RepoIDToCacheName(repoID))
	snapshot := filepath.Join(repoCache, "snapshots", "abc123")
	blobs := filepath.Join(repoCache, "blobs")

	safetensorsBlob := filepath.Join(blobs, "safetensorsblob")
	if err := os.WriteFile(safetensorsBlob, []byte("fake safetensors"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "blobs", "safetensorsblob"), filepath.Join(snapshot, "model.safetensors")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "blobs", "missingblob"), filepath.Join(snapshot, "broken.gguf")); err != nil {
		t.Fatal(err)
	}
}

func replaceGGUFBlob(t *testing.T, cache, repoID, architecture, modelType string) {
	t.Helper()
	blob := filepath.Join(cache, RepoIDToCacheName(repoID), "blobs", "ggufblob")
	if err := writeMinimalGGUF(blob, architecture, modelType); err != nil {
		t.Fatal(err)
	}
}

func addGGUFFile(t *testing.T, cache, repoID, name, architecture, modelType string) {
	t.Helper()
	repoCache := filepath.Join(cache, RepoIDToCacheName(repoID))
	blobName := strings.TrimSuffix(name, filepath.Ext(name)) + "-blob"
	blob := filepath.Join(repoCache, "blobs", blobName)
	if err := writeMinimalGGUF(blob, architecture, modelType); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "blobs", blobName), filepath.Join(repoCache, "snapshots", "abc123", name)); err != nil {
		t.Fatal(err)
	}
}

func writeMinimalGGUF(path, architecture, modelType string) error {
	var buffer bytes.Buffer
	buffer.WriteString("GGUF")
	for _, value := range []any{
		uint32(3),
		uint64(0),
		uint64(2),
	} {
		if err := binary.Write(&buffer, binary.LittleEndian, value); err != nil {
			return err
		}
	}
	writeGGUFString(&buffer, "general.architecture")
	if err := binary.Write(&buffer, binary.LittleEndian, uint32(ggufTypeString)); err != nil {
		return err
	}
	writeGGUFString(&buffer, architecture)
	writeGGUFString(&buffer, "general.type")
	if err := binary.Write(&buffer, binary.LittleEndian, uint32(ggufTypeString)); err != nil {
		return err
	}
	writeGGUFString(&buffer, modelType)
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func writeGGUFString(buffer *bytes.Buffer, value string) {
	_ = binary.Write(buffer, binary.LittleEndian, uint64(len(value)))
	buffer.WriteString(value)
}

func TestLMStudioLinksGGUFAndMetadata(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")

	summary, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeLMStudio,
		LinkMode:        LinkSymlink,
		IncludeMetadata: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	modelDir := filepath.Join(target, RuntimeLMStudio, "org", "Test-GGUF", "main")
	assertSymlink(t, filepath.Join(modelDir, "test.Q4_K_M.gguf"))
	assertSymlink(t, filepath.Join(modelDir, "README.md"))
	if summary.LinksCreated != 2 {
		t.Fatalf("LinksCreated = %d, want 2", summary.LinksCreated)
	}
}

func TestOllamaCreatesModelfileAndScript(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")

	summary, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeOllama,
		LinkMode:        LinkSymlink,
		IncludeMetadata: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	modelDir := filepath.Join(target, RuntimeOllama, "org--Test-GGUF", "q4_k_m")
	assertSymlink(t, filepath.Join(modelDir, "model.gguf"))

	modelfile, err := os.ReadFile(filepath.Join(modelDir, "Modelfile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(modelfile) != "FROM ./model.gguf\n" {
		t.Fatalf("Modelfile = %q", string(modelfile))
	}

	script, err := os.ReadFile(filepath.Join(target, RuntimeOllama, "create-all.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "ollama create 'org-test-gguf-q4_k_m'") {
		t.Fatalf("script missing ollama create command:\n%s", script)
	}
	if summary.OllamaModels != 1 {
		t.Fatalf("OllamaModels = %d, want 1", summary.OllamaModels)
	}
}

func TestLlamaCppOnlyFiltersNonGenerationGGUF(t *testing.T) {
	repoID := "org/Test-GGUF"
	cache := makeHFCache(t, repoID)
	replaceGGUFBlob(t, cache, repoID, "llama", "model")
	addGGUFFile(t, cache, repoID, "bert.Q8_0.gguf", "bert", "model")
	addGGUFFile(t, cache, repoID, "mmproj-vision.gguf", "clip", "model")
	addGGUFFile(t, cache, repoID, "mtp-draft.gguf", "llama", "model")
	target := filepath.Join(t.TempDir(), "target")

	summary, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeLMStudio,
		LinkMode:        LinkSymlink,
		IncludeMetadata: true,
		LlamaCppOnly:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	modelDir := filepath.Join(target, RuntimeLMStudio, "org", "Test-GGUF", "main")
	assertSymlink(t, filepath.Join(modelDir, "test.Q4_K_M.gguf"))
	assertSymlink(t, filepath.Join(modelDir, "mmproj-vision.gguf"))
	assertSymlink(t, filepath.Join(modelDir, "mtp-draft.gguf"))
	assertSymlink(t, filepath.Join(modelDir, "README.md"))
	if _, err := os.Lstat(filepath.Join(modelDir, "bert.Q8_0.gguf")); !os.IsNotExist(err) {
		t.Fatalf("bert GGUF was linked")
	}
	if summary.LinksCreated != 4 {
		t.Fatalf("LinksCreated = %d, want 4", summary.LinksCreated)
	}
}

func TestLlamaCppOnlyOllamaLinksCompanionGGUFsBesidePrimaryModel(t *testing.T) {
	repoID := "org/Test-GGUF"
	cache := makeHFCache(t, repoID)
	replaceGGUFBlob(t, cache, repoID, "llama", "model")
	addGGUFFile(t, cache, repoID, "mmproj-vision.gguf", "clip", "model")
	addGGUFFile(t, cache, repoID, "mtp-draft.gguf", "llama", "model")
	target := filepath.Join(t.TempDir(), "target")

	summary, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeOllama,
		LinkMode:        LinkSymlink,
		IncludeMetadata: true,
		LlamaCppOnly:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	modelDir := filepath.Join(target, RuntimeOllama, "org--Test-GGUF", "q4_k_m")
	assertSymlink(t, filepath.Join(modelDir, "model.gguf"))
	assertSymlink(t, filepath.Join(modelDir, "mmproj-vision.gguf"))
	assertSymlink(t, filepath.Join(modelDir, "mtp-draft.gguf"))
	if _, err := os.Stat(filepath.Join(target, RuntimeOllama, "org--Test-GGUF", "mmproj-vision")); !os.IsNotExist(err) {
		t.Fatalf("mmproj companion was created as a standalone Ollama model")
	}
	if _, err := os.Stat(filepath.Join(target, RuntimeOllama, "org--Test-GGUF", "mtp-draft")); !os.IsNotExist(err) {
		t.Fatalf("MTP companion was created as a standalone Ollama model")
	}
	if summary.OllamaModels != 1 {
		t.Fatalf("OllamaModels = %d, want 1", summary.OllamaModels)
	}
}

func TestLlamaCppOnlySkipsBertSnapshotAndOllamaSafetensorsFallback(t *testing.T) {
	repoID := "org/Test-GGUF"
	cache := makeHFCache(t, repoID)
	replaceGGUFBlob(t, cache, repoID, "bert", "model")
	addSafetensorsAndBrokenLink(t, cache, repoID)
	target := filepath.Join(t.TempDir(), "target")

	summary, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeOllama,
		LinkMode:        LinkSymlink,
		IncludeMetadata: true,
		LlamaCppOnly:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if summary.OllamaModels != 0 {
		t.Fatalf("OllamaModels = %d, want 0", summary.OllamaModels)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after strict filter skipped all models")
	}
}

func TestDryRunDoesNotCreateTarget(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")
	var out bytes.Buffer

	_, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeOllama,
		LinkMode:        LinkSymlink,
		IncludeMetadata: true,
		DryRun:          true,
		Stdout:          &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after dry run")
	}
	if !strings.Contains(out.String(), "symlink:") {
		t.Fatalf("dry run output = %q, want symlink operation", out.String())
	}
}

func TestJSONPlanRecordsOperationsAndDoesNotCreateTarget(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")

	summary, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeOllama,
		LinkMode:        LinkSymlink,
		IncludeMetadata: true,
		PlanFormat:      PlanJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after json plan")
	}
	if len(summary.Operations) == 0 {
		t.Fatalf("expected recorded operations")
	}
	if summary.Operations[0].Status != "planned" {
		t.Fatalf("operation status = %q, want planned", summary.Operations[0].Status)
	}
	if summary.TotalBytes == 0 {
		t.Fatalf("TotalBytes = 0, want referenced size")
	}
}

func TestCopyModeCopiesResolvedFile(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")

	_, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeLMStudio,
		LinkMode:        LinkCopy,
		IncludeMetadata: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	copied := filepath.Join(target, RuntimeLMStudio, "org", "Test-GGUF", "main", "test.Q4_K_M.gguf")
	info, err := os.Lstat(copied)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("copy destination is a symlink")
	}
	content, err := os.ReadFile(copied)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "fake gguf" {
		t.Fatalf("copied content = %q", string(content))
	}
}

func TestHardlinkMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink behavior differs on Windows")
	}
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")

	_, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeLMStudio,
		LinkMode:        LinkHard,
		IncludeMetadata: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	hardlink := filepath.Join(target, RuntimeLMStudio, "org", "Test-GGUF", "main", "test.Q4_K_M.gguf")
	info, err := os.Lstat(hardlink)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("hardlink destination is a symlink")
	}
}

func TestConflictPolicyFail(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")
	existing := filepath.Join(target, RuntimeLMStudio, "org", "Test-GGUF", "main", "test.Q4_K_M.gguf")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeLMStudio,
		LinkMode:        LinkSymlink,
		ConflictPolicy:  ConflictFail,
		IncludeMetadata: false,
	})
	if err == nil || !strings.Contains(err.Error(), "destination exists") {
		t.Fatalf("err = %v, want destination exists", err)
	}
}

func TestConflictPolicyReplace(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")
	existing := filepath.Join(target, RuntimeLMStudio, "org", "Test-GGUF", "main", "test.Q4_K_M.gguf")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       target,
		Runtime:         RuntimeLMStudio,
		LinkMode:        LinkSymlink,
		ConflictPolicy:  ConflictReplace,
		IncludeMetadata: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSymlink(t, existing)
}

func TestListInventoryReportsFilesAndBrokenLinks(t *testing.T) {
	repoID := "org/Test-GGUF"
	cache := makeHFCache(t, repoID)
	addSafetensorsAndBrokenLink(t, cache, repoID)

	inventory, err := List(context.Background(), ListOptions{CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Repos) != 1 {
		t.Fatalf("repos = %d, want 1", len(inventory.Repos))
	}
	snapshot := inventory.Repos[0].Snapshots[0]
	if len(snapshot.GGUF) != 1 {
		t.Fatalf("gguf files = %d, want 1 healthy gguf", len(snapshot.GGUF))
	}
	if len(snapshot.Safetensors) != 1 {
		t.Fatalf("safetensors files = %d, want 1", len(snapshot.Safetensors))
	}
	if len(snapshot.Broken) != 1 {
		t.Fatalf("broken files = %d, want 1", len(snapshot.Broken))
	}
	if snapshot.TotalSize == 0 {
		t.Fatalf("TotalSize = 0, want positive size")
	}
}

func TestListLlamaCppOnlyReportsOnlyCompatibleGGUF(t *testing.T) {
	repoID := "org/Test-GGUF"
	cache := makeHFCache(t, repoID)
	replaceGGUFBlob(t, cache, repoID, "llama", "model")
	addGGUFFile(t, cache, repoID, "bert.Q8_0.gguf", "bert", "model")
	addGGUFFile(t, cache, repoID, "mmproj-vision.gguf", "clip", "model")

	inventory, err := List(context.Background(), ListOptions{CacheDir: cache, LlamaCppOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := inventory.Repos[0].Snapshots[0]
	if len(snapshot.GGUF) != 2 {
		t.Fatalf("gguf files = %d, want 1 compatible gguf and 1 companion", len(snapshot.GGUF))
	}
	seen := map[string]string{}
	for _, file := range snapshot.GGUF {
		seen[file.Name] = file.Architecture
	}
	if seen["test.Q4_K_M.gguf"] != "llama" {
		t.Fatalf("primary architecture = %q, want llama", seen["test.Q4_K_M.gguf"])
	}
	if _, ok := seen["mmproj-vision.gguf"]; !ok {
		t.Fatalf("missing mmproj companion from strict list")
	}
}

func TestOllamaNameTemplate(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	target := filepath.Join(t.TempDir(), "target")

	_, err := Run(context.Background(), Options{
		CacheDir:           cache,
		TargetDir:          target,
		Runtime:            RuntimeOllama,
		LinkMode:           LinkSymlink,
		IncludeMetadata:    true,
		OllamaNameTemplate: "local-{repo}-{quant}",
	})
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(target, RuntimeOllama, "create-all.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "ollama create 'local-org-test-gguf-q4_k_m'") {
		t.Fatalf("script missing templated model name:\n%s", script)
	}
}

func TestMissingRepoReturnsUsefulError(t *testing.T) {
	cache := makeHFCache(t, "org/Test-GGUF")
	_, err := Run(context.Background(), Options{
		CacheDir:        cache,
		TargetDir:       filepath.Join(t.TempDir(), "target"),
		Runtime:         RuntimeLMStudio,
		LinkMode:        LinkSymlink,
		Repos:           []string{"missing/repo"},
		IncludeMetadata: true,
	})
	if err == nil || !strings.Contains(err.Error(), "missing/repo") {
		t.Fatalf("err = %v, want missing repo error", err)
	}
}

func assertSymlink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", path)
	}
}
