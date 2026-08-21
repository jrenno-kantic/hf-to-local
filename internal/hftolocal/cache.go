package hftolocal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// discoverRepos returns every model repo in the cache, unfiltered. Selection is
// a separate step (filterRepos) so --resolve-names can rewrite ids to their
// canonical form in between: filtering first would compare a user-supplied
// canonical id against the legacy org-less folder name and find nothing.
func discoverRepos(cacheDir string) ([]CachedRepo, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("read HF cache directory %s: %w", cacheDir, err)
	}

	var repos []CachedRepo
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "models--") {
			continue
		}
		repos = append(repos, CachedRepo{
			RepoID:   CacheNameToRepoID(entry.Name()),
			CacheDir: filepath.Join(cacheDir, entry.Name()),
		})
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].RepoID < repos[j].RepoID
	})
	return repos, nil
}

// filterRepos keeps the repos matching requested. A repo matches on its current
// id or on the id its cache folder encodes, so --repo accepts either the legacy
// org-less name or the canonical one once --resolve-names has run.
func filterRepos(repos []CachedRepo, requested []string, resolveNames bool) ([]CachedRepo, error) {
	if len(requested) == 0 {
		return repos, nil
	}

	requestedSet := make(map[string]struct{}, len(requested))
	for _, repo := range requested {
		requestedSet[repo] = struct{}{}
	}

	var selected []CachedRepo
	matched := make(map[string]struct{}, len(requested))
	for _, repo := range repos {
		cachedID := CacheNameToRepoID(filepath.Base(repo.CacheDir))
		hit := false
		for _, candidate := range []string{repo.RepoID, cachedID} {
			if _, ok := requestedSet[candidate]; ok {
				matched[candidate] = struct{}{}
				hit = true
			}
		}
		if hit {
			selected = append(selected, repo)
		}
	}

	var missing []string
	for _, repo := range requested {
		if _, ok := matched[repo]; !ok {
			missing = append(missing, repo)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		message := fmt.Sprintf("requested repo(s) not found in cache: %s", strings.Join(missing, ", "))
		if !resolveNames {
			message += " (repos cached under a legacy org-less id match only with --resolve-names)"
		}
		return nil, errors.New(message)
	}

	return selected, nil
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
