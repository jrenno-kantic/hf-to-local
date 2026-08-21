package hftolocal

import (
	"context"
	"path/filepath"
)

// targetBase builds the destination root for a snapshot. Every element is
// sanitized: cache folder names decode "--" back to "/", so an unsanitized
// namespace or name can contain ".." and escape opts.TargetDir.
func targetBase(opts Options, runtime string, snapshot Snapshot) string {
	namespace, name := repoParts(snapshot.Repo.RepoID)
	return filepath.Join(
		opts.TargetDir,
		runtime,
		safeSegment(namespace),
		safeRelPath(name),
		safeSegment(snapshot.RevisionLabel),
	)
}

func createMirror(ctx context.Context, snapshot Snapshot, opts Options, summary *Summary) error {
	base := targetBase(opts, RuntimeMirror, snapshot)
	return walkFiles(snapshot.Path, func(source string) error {
		if err := checkCanceled(ctx); err != nil {
			return err
		}
		rel, err := filepath.Rel(snapshot.Path, source)
		if err != nil {
			return err
		}
		return applyLink(source, filepath.Join(base, rel), opts, summary)
	})
}

func createLMStudio(ctx context.Context, snapshot Snapshot, opts Options, summary *Summary, classifier *ggufClassifier) error {
	base := targetBase(opts, RuntimeLMStudio, snapshot)
	return walkFiles(snapshot.Path, func(source string) error {
		if err := checkCanceled(ctx); err != nil {
			return err
		}
		if !classifier.shouldIncludeGGUF(source, opts.LlamaCppOnly) && !(opts.IncludeMetadata && shouldLinkMetadata(source)) {
			return nil
		}
		rel, err := filepath.Rel(snapshot.Path, source)
		if err != nil {
			return err
		}
		return applyLink(source, filepath.Join(base, rel), opts, summary)
	})
}
