package hftolocal

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type ListOptions struct {
	CacheDir     string
	Repos        []string
	Revision     string
	AllSnapshots bool
	LlamaCppOnly bool
	ResolveNames bool
}

type Inventory struct {
	CacheDir string          `json:"cache_dir"`
	Repos    []RepoInventory `json:"repos"`
}

type RepoInventory struct {
	RepoID           string              `json:"repo_id"`
	CacheDir         string              `json:"cache_dir"`
	SelectedRevision string              `json:"selected_revision,omitempty"`
	SelectedCommit   string              `json:"selected_commit,omitempty"`
	Snapshots        []SnapshotInventory `json:"snapshots"`
}

type SnapshotInventory struct {
	RevisionLabel string          `json:"revision_label"`
	Commit        string          `json:"commit"`
	Path          string          `json:"path"`
	GGUF          []FileInventory `json:"gguf"`
	Safetensors   []FileInventory `json:"safetensors"`
	Metadata      []FileInventory `json:"metadata"`
	Broken        []BrokenFile    `json:"broken"`
	TotalSize     int64           `json:"total_size"`

	hasCompatiblePrimaryGGUF bool
}

type FileInventory struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Architecture string `json:"architecture,omitempty"`
	Type         string `json:"type,omitempty"`
}

type BrokenFile struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

func List(opts ListOptions) (Inventory, error) {
	if opts.CacheDir == "" {
		opts.CacheDir = DefaultHFCache()
	}
	cacheDir, err := expandPath(opts.CacheDir)
	if err != nil {
		return Inventory{}, err
	}
	opts.CacheDir = cacheDir

	repos, err := discoverRepos(opts.CacheDir, opts.Repos)
	if err != nil {
		return Inventory{}, err
	}

	if opts.ResolveNames {
		resolveRepoNames(repos, defaultNameResolver, defaultNameCachePath(), resolveTimeout)
	}

	inventory := Inventory{CacheDir: opts.CacheDir}
	ggufClassifier := newGGUFClassifier()
	for _, repo := range repos {
		repoInventory, err := inspectRepo(repo, opts, ggufClassifier)
		if err != nil {
			return inventory, err
		}
		inventory.Repos = append(inventory.Repos, repoInventory)
	}
	return inventory, nil
}

func inspectRepo(repo CachedRepo, opts ListOptions, classifier *ggufClassifier) (RepoInventory, error) {
	repoInventory := RepoInventory{
		RepoID:   repo.RepoID,
		CacheDir: repo.CacheDir,
	}

	commit, label, err := resolveRevision(repo.CacheDir, opts.Revision)
	if err == nil {
		repoInventory.SelectedCommit = commit
		repoInventory.SelectedRevision = label
	}

	snapshotsDir := filepath.Join(repo.CacheDir, "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return repoInventory, nil
		}
		return repoInventory, err
	}

	var snapshots []Snapshot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		snapshot := Snapshot{
			Repo:          repo,
			RevisionLabel: entry.Name(),
			Commit:        entry.Name(),
			Path:          filepath.Join(snapshotsDir, entry.Name()),
		}
		if opts.AllSnapshots || opts.Revision == "" || entry.Name() == commit {
			snapshots = append(snapshots, snapshot)
		}
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Commit < snapshots[j].Commit
	})

	for _, snapshot := range snapshots {
		snapshotInventory, err := inspectSnapshot(snapshot, opts, classifier)
		if err != nil {
			return repoInventory, err
		}
		if opts.LlamaCppOnly && !snapshotInventory.hasCompatiblePrimaryGGUF {
			continue
		}
		repoInventory.Snapshots = append(repoInventory.Snapshots, snapshotInventory)
	}
	return repoInventory, nil
}

func inspectSnapshot(snapshot Snapshot, opts ListOptions, classifier *ggufClassifier) (SnapshotInventory, error) {
	inventory := SnapshotInventory{
		RevisionLabel: snapshot.RevisionLabel,
		Commit:        snapshot.Commit,
		Path:          snapshot.Path,
	}
	err := walkFiles(snapshot.Path, func(source string) error {
		resolved, size, err := resolveSource(source)
		if err != nil {
			inventory.Broken = append(inventory.Broken, BrokenFile{Path: source, Error: err.Error()})
			return nil
		}
		file := FileInventory{
			Path: resolved,
			Name: filepath.Base(source),
			Size: size,
		}
		switch {
		case isGGUF(source):
			metadata, compatible := classifier.classifyGGUF(source)
			if compatible && !isCompanionGGUF(source) {
				inventory.hasCompatiblePrimaryGGUF = true
			}
			if opts.LlamaCppOnly && !compatible && !isCompanionGGUF(source) {
				return nil
			}
			file.Architecture = metadata.Architecture
			file.Type = metadata.Type
			if isCompanionGGUF(source) && file.Type == "" {
				file.Type = "companion"
			}
			inventory.GGUF = append(inventory.GGUF, file)
			inventory.TotalSize += size
		case isSafetensors(source):
			if opts.LlamaCppOnly {
				return nil
			}
			inventory.Safetensors = append(inventory.Safetensors, file)
			inventory.TotalSize += size
		case shouldLinkMetadata(source):
			if opts.LlamaCppOnly {
				return nil
			}
			inventory.Metadata = append(inventory.Metadata, file)
			inventory.TotalSize += size
		}
		return nil
	})
	return inventory, err
}
