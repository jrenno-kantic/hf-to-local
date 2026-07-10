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
		return err
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

func applyWrite(destination, content string, opts Options, summary *Summary) (bool, error) {
	op := linkOperation{
		destination: destination,
		action:      "write",
		size:        int64(len(content)),
		content:     content,
	}
	ok, err := materializeWrite(op, opts, summary)
	if err != nil {
		return false, err
	}
	if ok {
		summary.FilesWritten++
	} else {
		summary.FilesSkipped++
	}
	return ok, nil
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

func materializeWrite(op linkOperation, opts Options, summary *Summary) (bool, error) {
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
		return true, nil
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
			return false, nil
		case ConflictFail:
			return false, fmt.Errorf("destination exists: %s", op.destination)
		case ConflictReplace:
			if err := os.Remove(op.destination); err != nil {
				return false, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(op.destination), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(op.destination, []byte(op.content), 0o644); err != nil {
		return false, err
	}
	recordOperation(opts, summary, Operation{
		Type:        "write",
		Destination: op.destination,
		Size:        op.size,
		Status:      "created",
	})
	return true, nil
}

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

	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, sourceInfo.Mode().Perm())
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Close()
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
