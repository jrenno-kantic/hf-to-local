package hftolocal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

type linkOperation struct {
	source      string
	destination string
	action      string
	size        int64
	content     string
}

// applyLink materializes one source file into the target tree. A source that
// cannot be resolved is counted, reported, and skipped rather than aborting the
// run: interrupted downloads routinely leave dangling snapshot->blob symlinks in
// the HF cache, and one of them must not make the whole cache unusable. The
// count surfaces in the final summary as broken_sources.
func applyLink(source, destination string, opts Options, summary *Summary) error {
	resolvedSource, size, err := resolveSource(source)
	if err != nil {
		summary.BrokenSources++
		recordOperation(opts, summary, Operation{
			Type:    "link",
			Action:  opts.LinkMode,
			Source:  source,
			Status:  "broken",
			Message: err.Error(),
		})
		warn(opts, "skipping broken source %s: %v", source, err)
		return nil
	}
	op := linkOperation{
		source:      resolvedSource,
		destination: destination,
		action:      opts.LinkMode,
		size:        size,
	}
	ok, err := materialize(op, opts, summary)
	if err != nil {
		return err
	}
	if ok {
		summary.LinksCreated++
		summary.TotalBytes += size
	} else {
		summary.FilesSkipped++
	}
	return nil
}

// applyWrite materializes generated content. It reports two independent facts:
// created is true when this run wrote the file, present is true whenever the
// file exists afterwards -- including when an existing file was left alone under
// --on-conflict skip. Callers that build follow-up actions (Ollama imports) must
// key off present, so re-running against an already-populated target still finds
// its models.
func applyWrite(destination, content string, opts Options, summary *Summary) (created bool, present bool, err error) {
	op := linkOperation{
		destination: destination,
		action:      "write",
		size:        int64(len(content)),
		content:     content,
	}
	created, present, err = materializeWrite(op, opts, summary)
	if err != nil {
		return false, false, err
	}
	if created {
		summary.FilesWritten++
	} else {
		summary.FilesSkipped++
	}
	return created, present, nil
}

func resolveSource(source string) (string, int64, error) {
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", 0, fmt.Errorf("resolve source %s: %w", source, err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", 0, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", 0, err
	}
	return absolute, info.Size(), nil
}

func materialize(op linkOperation, opts Options, summary *Summary) (bool, error) {
	if opts.DryRun {
		if opts.PlanFormat != PlanJSON {
			fmt.Fprintf(opts.Stdout, "%s: %s -> %s\n", op.action, op.source, op.destination)
		}
		recordOperation(opts, summary, Operation{
			Type:        "link",
			Action:      op.action,
			Source:      op.source,
			Destination: op.destination,
			Size:        op.size,
			Status:      "planned",
		})
		return true, nil
	}

	if _, err := os.Lstat(op.destination); err == nil {
		switch opts.ConflictPolicy {
		case ConflictSkip:
			recordOperation(opts, summary, Operation{
				Type:        "link",
				Action:      op.action,
				Source:      op.source,
				Destination: op.destination,
				Size:        op.size,
				Status:      "skipped",
				Message:     "destination exists",
			})
			return false, nil
		case ConflictFail:
			return false, fmt.Errorf("destination exists: %s", op.destination)
		case ConflictReplace:
			if err := os.RemoveAll(op.destination); err != nil {
				return false, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(op.destination), 0o755); err != nil {
		return false, err
	}

	switch op.action {
	case LinkSymlink:
		if err := os.Symlink(op.source, op.destination); err != nil {
			return false, platformLinkError(op.action, op.source, op.destination, err)
		}
	case LinkHard:
		if err := os.Link(op.source, op.destination); err != nil {
			return false, platformLinkError(op.action, op.source, op.destination, err)
		}
	case LinkCopy:
		if err := copyFile(op.source, op.destination); err != nil {
			return false, err
		}
	default:
		return false, fmt.Errorf("unsupported link action: %s", op.action)
	}

	recordOperation(opts, summary, Operation{
		Type:        "link",
		Action:      op.action,
		Source:      op.source,
		Destination: op.destination,
		Size:        op.size,
		Status:      "created",
	})
	return true, nil
}

func materializeWrite(op linkOperation, opts Options, summary *Summary) (created bool, present bool, err error) {
	if opts.DryRun {
		if opts.PlanFormat != PlanJSON {
			fmt.Fprintf(opts.Stdout, "write: %s\n", op.destination)
		}
		recordOperation(opts, summary, Operation{
			Type:        "write",
			Destination: op.destination,
			Size:        op.size,
			Status:      "planned",
		})
		return true, true, nil
	}
	if _, err := os.Lstat(op.destination); err == nil {
		switch opts.ConflictPolicy {
		case ConflictSkip:
			recordOperation(opts, summary, Operation{
				Type:        "write",
				Destination: op.destination,
				Size:        op.size,
				Status:      "skipped",
				Message:     "destination exists",
			})
			return false, true, nil
		case ConflictFail:
			return false, false, fmt.Errorf("destination exists: %s", op.destination)
		case ConflictReplace:
			if err := os.Remove(op.destination); err != nil {
				return false, false, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, false, err
	}

	if err := os.MkdirAll(filepath.Dir(op.destination), 0o755); err != nil {
		return false, false, err
	}
	if err := os.WriteFile(op.destination, []byte(op.content), 0o644); err != nil {
		return false, false, err
	}
	recordOperation(opts, summary, Operation{
		Type:        "write",
		Destination: op.destination,
		Size:        op.size,
		Status:      "created",
	})
	return true, true, nil
}

// copyFile writes through a temporary file in the destination directory and
// renames it into place. Model files are large, so an interrupted copy is a real
// possibility; without the rename a partial file would be indistinguishable from
// a complete one and --on-conflict skip would keep it on the next run.
func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	sourceInfo, err := input.Stat()
	if err != nil {
		return err
	}

	output, err := os.CreateTemp(filepath.Dir(destination), ".hf-to-local-*.part")
	if err != nil {
		return err
	}
	temp := output.Name()
	defer func() {
		output.Close()
		os.Remove(temp) // no-op once the rename below has succeeded
	}()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temp, sourceInfo.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(temp, destination)
}

func platformLinkError(action, source, destination string, err error) error {
	switch action {
	case LinkSymlink:
		if runtime.GOOS == "windows" {
			return fmt.Errorf("create symlink %s -> %s: %w; on Windows, enable Developer Mode or run with privileges", destination, source, err)
		}
		return fmt.Errorf("create symlink %s -> %s: %w", destination, source, err)
	case LinkHard:
		if errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("create hardlink %s -> %s: %w; hardlinks cannot cross filesystems, use --link-mode symlink or --link-mode copy", destination, source, err)
		}
		return fmt.Errorf("create hardlink %s -> %s: %w", destination, source, err)
	default:
		return err
	}
}
