package hftolocal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func createOllama(ctx context.Context, snapshot Snapshot, opts Options, summary *Summary, classifier *ggufClassifier) ([]OllamaCreate, error) {
	var sources []string
	if err := walkFiles(snapshot.Path, func(source string) error {
		if err := checkCanceled(ctx); err != nil {
			return err
		}
		sources = append(sources, source)
		return nil
	}); err != nil {
		return nil, err
	}

	var ggufFiles []string
	var companionGGUFs []string
	var safetensorsFiles []string
	for _, source := range sources {
		if classifier.shouldIncludePrimaryGGUF(source, opts.LlamaCppOnly) {
			ggufFiles = append(ggufFiles, source)
		} else if classifier.shouldIncludeGGUF(source, opts.LlamaCppOnly) {
			companionGGUFs = append(companionGGUFs, source)
		}
		if isSafetensors(source) {
			safetensorsFiles = append(safetensorsFiles, source)
		}
	}

	repoSlug := safeSegment(strings.ReplaceAll(snapshot.Repo.RepoID, "/", "--"))
	revisionSlug := safeSegment(snapshot.RevisionLabel)
	var creates []OllamaCreate

	for _, gguf := range ggufFiles {
		quant := quantName(gguf)
		modelDir := filepath.Join(opts.TargetDir, RuntimeOllama, repoSlug, quant)
		modelName := modelNameFromTemplate(snapshot.Repo.RepoID, quant, snapshot.RevisionLabel, opts.OllamaNameTemplate)
		if err := applyLink(gguf, filepath.Join(modelDir, "model.gguf"), opts, summary); err != nil {
			return nil, err
		}
		for _, companion := range companionGGUFs {
			if err := applyLink(companion, filepath.Join(modelDir, filepath.Base(companion)), opts, summary); err != nil {
				return nil, err
			}
		}
		if _, present, err := applyWrite(filepath.Join(modelDir, "Modelfile"), "FROM ./model.gguf\n", opts, summary); err != nil {
			return nil, err
		} else if present {
			summary.OllamaModels++
			creates = append(creates, OllamaCreate{ModelName: modelName, ModelDir: modelDir})
		}
	}

	if !opts.LlamaCppOnly && len(safetensorsFiles) > 0 && len(ggufFiles) == 0 {
		modelDir := filepath.Join(opts.TargetDir, RuntimeOllama, repoSlug, "safetensors-"+revisionSlug)
		for _, source := range sources {
			rel, err := filepath.Rel(snapshot.Path, source)
			if err != nil {
				return nil, err
			}
			if err := applyLink(source, filepath.Join(modelDir, rel), opts, summary); err != nil {
				return nil, err
			}
		}
		modelName := modelNameFromTemplate(snapshot.Repo.RepoID, revisionSlug, snapshot.RevisionLabel, opts.OllamaNameTemplate)
		if _, present, err := applyWrite(filepath.Join(modelDir, "Modelfile"), "FROM .\n", opts, summary); err != nil {
			return nil, err
		} else if present {
			summary.OllamaModels++
			creates = append(creates, OllamaCreate{ModelName: modelName, ModelDir: modelDir})
		}
	}

	return creates, nil
}

func writeOllamaScript(targetDir string, creates []OllamaCreate, opts Options, summary *Summary) error {
	if len(creates) == 0 {
		return nil
	}
	commands := make([]string, 0, len(creates))
	for _, create := range creates {
		commands = append(commands, fmt.Sprintf("cd %s && ollama create %s -f Modelfile", shellQuote(create.ModelDir), shellQuote(create.ModelName)))
	}
	content := "#!/usr/bin/env bash\nset -euo pipefail\n\n" + strings.Join(commands, "\n") + "\n"
	script := filepath.Join(targetDir, RuntimeOllama, "create-all.sh")
	if created, _, err := applyWrite(script, content, opts, summary); err != nil {
		return err
	} else if created && !opts.DryRun {
		if err := os.Chmod(script, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func modelNameFromTemplate(repoID, quant, revision, template string) string {
	if template == "" {
		return modelNameFromPath(repoID, quant)
	}
	repo := strings.ToLower(safeSegment(strings.ReplaceAll(repoID, "/", "-")))
	repoSlug := strings.ToLower(safeSegment(strings.ReplaceAll(repoID, "/", "--")))
	replacer := strings.NewReplacer(
		"{repo}", repo,
		"{repo_slug}", repoSlug,
		"{quant}", strings.ToLower(safeSegment(quant)),
		"{revision}", strings.ToLower(safeSegment(revision)),
	)
	return strings.ToLower(safeSegment(replacer.Replace(template)))
}

func runOllamaCreate(ctx context.Context, create OllamaCreate) error {
	command := exec.CommandContext(ctx, "ollama", "create", create.ModelName, "-f", "Modelfile")
	command.Dir = create.ModelDir
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ollama create %s failed: %w\n%s", create.ModelName, err, strings.TrimSpace(string(output)))
	}
	return nil
}
