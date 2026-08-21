package hftolocal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func DefaultHFCache() string {
	if value := os.Getenv("HF_HUB_CACHE"); value != "" {
		return value
	}
	if value := os.Getenv("HF_HOME"); value != "" {
		return filepath.Join(value, "hub")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".cache", "huggingface", "hub")
	}
	return filepath.Join(home, ".cache", "huggingface", "hub")
}

func Run(ctx context.Context, opts Options) (Summary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateOptions(&opts); err != nil {
		return Summary{}, err
	}

	cacheDir, err := expandPath(opts.CacheDir)
	if err != nil {
		return Summary{}, err
	}
	targetDir, err := expandPath(opts.TargetDir)
	if err != nil {
		return Summary{}, err
	}
	opts.CacheDir = cacheDir
	opts.TargetDir = targetDir

	repos, err := discoverRepos(opts.CacheDir)
	if err != nil {
		return Summary{}, err
	}
	if opts.ResolveNames {
		resolveRepoNames(ctx, repos, defaultNameResolver, defaultNameCachePath(), resolveTimeout, resolveBudget)
	}
	repos, err = filterRepos(repos, opts.Repos, opts.ResolveNames)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{ReposSeen: len(repos)}
	var ollamaCreates []OllamaCreate
	ggufClassifier := newGGUFClassifier()

	for index, repo := range repos {
		if err := checkCanceled(ctx); err != nil {
			return summary, err
		}
		snapshots, err := selectSnapshots(repo, opts.Revision, opts.AllSnapshots)
		if err != nil {
			return summary, err
		}
		summary.SnapshotsSeen += len(snapshots)
		progress(opts, "[%d/%d] %s (%d snapshot(s))", index+1, len(repos), repo.RepoID, len(snapshots))

		for _, snapshot := range snapshots {
			if err := checkCanceled(ctx); err != nil {
				return summary, err
			}
			if opts.LlamaCppOnly {
				ok, err := snapshotHasCompatibleGGUF(snapshot, ggufClassifier)
				if err != nil {
					return summary, err
				}
				if !ok {
					continue
				}
			}
			if opts.Runtime == RuntimeAll || opts.Runtime == RuntimeMirror {
				if err := createMirror(ctx, snapshot, opts, &summary); err != nil {
					return summary, err
				}
			}
			if opts.Runtime == RuntimeAll || opts.Runtime == RuntimeLMStudio {
				if err := createLMStudio(ctx, snapshot, opts, &summary, ggufClassifier); err != nil {
					return summary, err
				}
			}
			if opts.Runtime == RuntimeAll || opts.Runtime == RuntimeOllama {
				creates, err := createOllama(ctx, snapshot, opts, &summary, ggufClassifier)
				if err != nil {
					return summary, err
				}
				ollamaCreates = append(ollamaCreates, creates...)
			}
		}
	}

	if opts.Runtime == RuntimeAll || opts.Runtime == RuntimeOllama {
		if err := writeOllamaScript(opts.TargetDir, ollamaCreates, opts, &summary); err != nil {
			return summary, err
		}
		if opts.CreateOllama && !opts.DryRun {
			for _, create := range ollamaCreates {
				if err := checkCanceled(ctx); err != nil {
					return summary, err
				}
				progress(opts, "ollama create %s", create.ModelName)
				if err := runOllamaCreate(ctx, create); err != nil {
					return summary, err
				}
				summary.OllamaCreatesRun++
				recordOperation(opts, &summary, Operation{
					Type:        "ollama-create",
					Destination: create.ModelName,
					Status:      "created",
					Message:     create.ModelDir,
				})
			}
		}
	}

	return summary, nil
}

func validateOptions(opts *Options) error {
	if opts.TargetDir == "" {
		return errors.New("--target is required")
	}
	if opts.CacheDir == "" {
		opts.CacheDir = DefaultHFCache()
	}
	if opts.Runtime == "" {
		opts.Runtime = RuntimeAll
	}
	if opts.LinkMode == "" {
		opts.LinkMode = LinkSymlink
	}
	if opts.Overwrite {
		opts.ConflictPolicy = ConflictReplace
	}
	if opts.ConflictPolicy == "" {
		opts.ConflictPolicy = ConflictSkip
	}
	if opts.PlanFormat != "" {
		opts.DryRun = true
	}
	if opts.OllamaNameTemplate == "" {
		opts.OllamaNameTemplate = "{repo}-{quant}"
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}

	switch opts.Runtime {
	case RuntimeAll, RuntimeLMStudio, RuntimeOllama, RuntimeMirror:
	default:
		return fmt.Errorf("unsupported runtime %q", opts.Runtime)
	}
	switch opts.LinkMode {
	case LinkSymlink, LinkHard, LinkCopy:
	default:
		return fmt.Errorf("unsupported link mode %q", opts.LinkMode)
	}
	switch opts.ConflictPolicy {
	case ConflictSkip, ConflictReplace, ConflictFail:
	default:
		return fmt.Errorf("unsupported conflict policy %q", opts.ConflictPolicy)
	}
	switch opts.PlanFormat {
	case "", PlanText, PlanJSON:
	default:
		return fmt.Errorf("unsupported plan format %q", opts.PlanFormat)
	}
	return nil
}
