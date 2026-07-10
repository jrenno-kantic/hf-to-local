package hftolocal

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var quantSplitRE = regexp.MustCompile(`[._-]+`)

func walkFiles(root string, visit func(source string) error) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
			return visit(path)
		}
		return nil
	})
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func isGGUF(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".gguf")
}

func isSafetensors(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".safetensors")
}

func shouldLinkMetadata(path string) bool {
	_, ok := metadataNames[filepath.Base(path)]
	return ok
}

func repoParts(repoID string) (string, string) {
	parts := strings.SplitN(repoID, "/", 2)
	if len(parts) == 1 {
		return "_", parts[0]
	}
	return parts[0], parts[1]
}

func safeSegment(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	cleaned := strings.Trim(builder.String(), "-")
	if cleaned == "" {
		return "model"
	}
	return cleaned
}

func quantName(ggufPath string) string {
	stem := strings.TrimSuffix(filepath.Base(ggufPath), filepath.Ext(ggufPath))
	parts := nonEmpty(quantSplitRE.Split(stem, -1))
	for i := len(parts) - 1; i >= 0; i-- {
		token := strings.ToUpper(parts[i])
		if strings.HasPrefix(token, "Q") || token == "F16" || token == "F32" {
			return safeSegment(strings.ToLower(strings.Join(parts[i:], "_")))
		}
	}
	return safeSegment(strings.ToLower(stem))
}

func nonEmpty(parts []string) []string {
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func modelNameFromPath(repoID, suffix string) string {
	base := strings.ToLower(safeSegment(strings.ReplaceAll(repoID, "/", "-")))
	if suffix == "" {
		return base
	}
	return base + "-" + strings.ToLower(safeSegment(suffix))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func recordOperation(opts Options, summary *Summary, operation Operation) {
	if opts.PlanFormat == PlanJSON || opts.DryRun {
		summary.Operations = append(summary.Operations, operation)
	}
}
