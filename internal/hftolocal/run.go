package hftolocal

import (
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

func Run(opts Options) (Summary, error) {
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

	repos, err := discoverRepos(opts.CacheDir, opts.Repos)
	if err != nil {
		return Summary{}, err
	}

	if opts.ResolveNames {
		resolveRepoNames(repos, defaultNameResolver, defaultNameCachePath(), resolveTimeout)
	}

	summary := Summary{ReposSeen: len(repos)}
	var ollamaCreates []OllamaCreate
	ggufClassifier := newGGUFClassifier()

	for _, repo := range repos {
		snapshots, err := selectSnapshots(repo, opts.Revision, opts.AllSnapshots)
		if err != nil {
			return summary, err
		}
		summary.SnapshotsSeen += len(snapshots)

		for _, snapshot := range snapshots {
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
				if err := createMirror(snapshot, opts, &summary); err != nil {
					return summary, err
				}
			}
			if opts.Runtime == RuntimeAll || opts.Runtime == RuntimeLMStudio {
				if err := createLMStudio(snapshot, opts, &summary, ggufClassifier); err != nil {
					return summary, err
				}
			}
			if opts.Runtime == RuntimeAll || opts.Runtime == RuntimeOllama {
				creates, err := createOllama(snapshot, opts, &summary, ggufClassifier)
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
				if err := runOllamaCreate(create); err != nil {
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
