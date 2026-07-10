package hftolocal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func discoverRepos(cacheDir string, requested []string) ([]CachedRepo, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("read HF cache directory %s: %w", cacheDir, err)
	}

	requestedSet := make(map[string]struct{}, len(requested))
	for _, repo := range requested {
		requestedSet[repo] = struct{}{}
	}

	var repos []CachedRepo
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "models--") {
			continue
		}
		repoID := CacheNameToRepoID(entry.Name())
		if len(requestedSet) > 0 {
			if _, ok := requestedSet[repoID]; !ok {
				continue
			}
		}
		repos = append(repos, CachedRepo{
			RepoID:   repoID,
			CacheDir: filepath.Join(cacheDir, entry.Name()),
		})
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].RepoID < repos[j].RepoID
	})

	found := make(map[string]struct{}, len(repos))
	for _, repo := range repos {
		found[repo.RepoID] = struct{}{}
	}
	var missing []string
	for _, repo := range requested {
		if _, ok := found[repo]; !ok {
			missing = append(missing, repo)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("requested repo(s) not found in cache: %s", strings.Join(missing, ", "))
	}

	return repos, nil
}

func CacheNameToRepoID(name string) string {
	return strings.ReplaceAll(strings.TrimPrefix(name, "models--"), "--", "/")
}

func RepoIDToCacheName(repoID string) string {
	return "models--" + strings.ReplaceAll(repoID, "/", "--")
}

func selectSnapshots(repo CachedRepo, revision string, allSnapshots bool) ([]Snapshot, error) {
	snapshotsDir := filepath.Join(repo.CacheDir, "snapshots")
	info, err := os.Stat(snapshotsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	if allSnapshots {
		entries, err := os.ReadDir(snapshotsDir)
		if err != nil {
			return nil, err
		}
		var snapshots []Snapshot
		for _, entry := range entries {
			if entry.IsDir() {
				snapshots = append(snapshots, Snapshot{
					Repo:          repo,
					RevisionLabel: entry.Name(),
					Commit:        entry.Name(),
					Path:          filepath.Join(snapshotsDir, entry.Name()),
				})
			}
		}
		sort.Slice(snapshots, func(i, j int) bool {
			return snapshots[i].Commit < snapshots[j].Commit
		})
		return snapshots, nil
	}

	commit, label, err := resolveRevision(repo.CacheDir, revision)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(snapshotsDir, commit)
	info, err = os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	return []Snapshot{{Repo: repo, RevisionLabel: label, Commit: commit, Path: path}}, nil
}

func resolveRevision(cacheRepoDir, revision string) (commit string, label string, err error) {
	snapshotsDir := filepath.Join(cacheRepoDir, "snapshots")
	refsDir := filepath.Join(cacheRepoDir, "refs")

	if revision != "" {
		refPath := filepath.Join(refsDir, filepath.FromSlash(revision))
		content, readErr := os.ReadFile(refPath)
		if readErr == nil {
			return strings.TrimSpace(string(content)), revision, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return "", "", readErr
		}
		return revision, revision, nil
	}

	mainRef := filepath.Join(refsDir, "main")
	content, readErr := os.ReadFile(mainRef)
	if readErr == nil {
		return strings.TrimSpace(string(content)), "main", nil
	}
	if !errors.Is(readErr, os.ErrNotExist) {
		return "", "", readErr
	}

	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return "", "", err
	}
	var newestName string
	var newestModTime int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", "", err
		}
		modTime := info.ModTime().UnixNano()
		if newestName == "" || modTime > newestModTime {
			newestName = entry.Name()
			newestModTime = modTime
		}
	}
	if newestName == "" {
		return "", "", fmt.Errorf("no snapshots found in %s", snapshotsDir)
	}
	return newestName, newestName, nil
}
