package hftolocal

import "path/filepath"

func createMirror(snapshot Snapshot, opts Options, summary *Summary) error {
	namespace, name := repoParts(snapshot.Repo.RepoID)
	base := filepath.Join(opts.TargetDir, RuntimeMirror, namespace, name, safeSegment(snapshot.RevisionLabel))
	return walkFiles(snapshot.Path, func(source string) error {
		rel, err := filepath.Rel(snapshot.Path, source)
		if err != nil {
			return err
		}
		return applyLink(source, filepath.Join(base, rel), opts, summary)
	})
}

func createLMStudio(snapshot Snapshot, opts Options, summary *Summary, classifier *ggufClassifier) error {
	namespace, name := repoParts(snapshot.Repo.RepoID)
	base := filepath.Join(opts.TargetDir, RuntimeLMStudio, namespace, name, safeSegment(snapshot.RevisionLabel))
	return walkFiles(snapshot.Path, func(source string) error {
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
