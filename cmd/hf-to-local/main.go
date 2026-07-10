package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jrenno/hf-to-local/internal/hftolocal"
)

var version = "dev"

type repoFlags []string

func (r *repoFlags) String() string {
	return strings.Join(*r, ",")
}

func (r *repoFlags) Set(value string) error {
	if value == "" {
		return fmt.Errorf("repo cannot be empty")
	}
	*r = append(*r, value)
	return nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "list" {
		runList(os.Args[2:])
		return
	}
	runCreate(os.Args[1:])
}

func runCreate(args []string) {
	var repos repoFlags
	var opts hftolocal.Options
	var noIncludeMetadata bool
	var showVersion bool

	flags := flag.NewFlagSet("hf-to-local", flag.ExitOnError)
	flags.StringVar(&opts.CacheDir, "cache", hftolocal.DefaultHFCache(), "Hugging Face Hub cache directory")
	flags.StringVar(&opts.TargetDir, "target", "", "Base output directory for linked model folders")
	flags.StringVar(&opts.Runtime, "runtime", "all", "Output layout to create: all, lmstudio, ollama, mirror")
	flags.Var(&repos, "repo", "HF repo id to include, e.g. org/model. Can be passed multiple times")
	flags.StringVar(&opts.Revision, "revision", "", "HF ref or snapshot commit to use")
	flags.BoolVar(&opts.AllSnapshots, "all-snapshots", false, "Use every cached snapshot instead of selecting one snapshot per repo")
	flags.StringVar(&opts.LinkMode, "link-mode", "symlink", "How files are materialized: symlink, hardlink, copy")
	flags.StringVar(&opts.ConflictPolicy, "on-conflict", "skip", "Existing destination behavior: skip, replace, fail")
	flags.BoolVar(&opts.Overwrite, "overwrite", false, "Replace existing files or links in the target folders")
	flags.BoolVar(&opts.DryRun, "dry-run", false, "Print planned operations without writing files")
	flags.StringVar(&opts.PlanFormat, "plan", "", "Write a dry-run plan: text or json")
	flags.BoolVar(&opts.CreateOllama, "create", false, "Run ollama create for generated Ollama model folders")
	flags.StringVar(&opts.OllamaNameTemplate, "ollama-name-template", "{repo}-{quant}", "Ollama model name template using {repo}, {repo_slug}, {quant}, {revision}")
	flags.BoolVar(&noIncludeMetadata, "no-include-metadata", false, "Do not include small metadata files beside GGUF links")
	flags.BoolVar(&opts.LlamaCppOnly, "llama-cpp-only", false, "Only include GGUF text-generation model architectures recognized by llama.cpp")
	flags.BoolVar(&showVersion, "version", false, "Print version and exit")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s --target DIR [options]\n\n", flags.Name())
		fmt.Fprintf(flags.Output(), "       %s list [options]\n\n", flags.Name())
		fmt.Fprintln(flags.Output(), "Create local model folders from Hugging Face Hub cache snapshots for LM Studio and Ollama.")
		fmt.Fprintln(flags.Output(), "\nOptions:")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}
	if showVersion {
		fmt.Println(version)
		return
	}

	opts.Repos = repos
	opts.IncludeMetadata = !noIncludeMetadata
	opts.Stdout = os.Stdout

	summary, err := hftolocal.Run(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if opts.PlanFormat == hftolocal.PlanJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(summary); err != nil {
			fmt.Fprintf(os.Stderr, "error: write json plan: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf(
		"done: %d repo(s), %d snapshot(s), %d link(s), %d generated file(s), %d Ollama model definition(s), %d Ollama create(s), %d skipped, %s referenced, %d broken source(s)\n",
		summary.ReposSeen,
		summary.SnapshotsSeen,
		summary.LinksCreated,
		summary.FilesWritten,
		summary.OllamaModels,
		summary.OllamaCreatesRun,
		summary.FilesSkipped,
		humanBytes(summary.TotalBytes),
		summary.BrokenSources,
	)
}

func runList(args []string) {
	var repos repoFlags
	var opts hftolocal.ListOptions
	var jsonOutput bool

	flags := flag.NewFlagSet("hf-to-local list", flag.ExitOnError)
	flags.StringVar(&opts.CacheDir, "cache", hftolocal.DefaultHFCache(), "Hugging Face Hub cache directory")
	flags.Var(&repos, "repo", "HF repo id to include, e.g. org/model. Can be passed multiple times")
	flags.StringVar(&opts.Revision, "revision", "", "HF ref or snapshot commit to inspect")
	flags.BoolVar(&opts.AllSnapshots, "all-snapshots", false, "Show every cached snapshot when --revision is set")
	flags.BoolVar(&opts.LlamaCppOnly, "llama-cpp-only", false, "Only show GGUF text-generation model architectures recognized by llama.cpp")
	flags.BoolVar(&opts.ResolveNames, "resolve-names", false, "Resolve org-less cached repo ids to their canonical HF id via the Hugging Face API (requires network; falls back to the cached name when offline)")
	flags.BoolVar(&jsonOutput, "json", false, "Print JSON inventory")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [options]\n\n", flags.Name())
		fmt.Fprintln(flags.Output(), "List Hugging Face cache model snapshots and local files.")
		fmt.Fprintln(flags.Output(), "\nOptions:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}

	opts.Repos = repos
	inventory, err := hftolocal.List(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(inventory); err != nil {
			fmt.Fprintf(os.Stderr, "error: write json inventory: %v\n", err)
			os.Exit(1)
		}
		return
	}
	printInventory(inventory)
}

func printInventory(inventory hftolocal.Inventory) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "REPO\tREVISION\tGGUF\tSAFETENSORS\tMETADATA\tSIZE\tBROKEN\n")
	for _, repo := range inventory.Repos {
		for _, snapshot := range repo.Snapshots {
			label := snapshot.RevisionLabel
			if snapshot.Commit == repo.SelectedCommit {
				label += " *"
			}
			fmt.Fprintf(
				writer,
				"%s\t%s\t%d\t%d\t%d\t%s\t%d\n",
				repo.RepoID,
				label,
				len(snapshot.GGUF),
				len(snapshot.Safetensors),
				len(snapshot.Metadata),
				humanBytes(snapshot.TotalSize),
				len(snapshot.Broken),
			)
		}
	}
	writer.Flush()
}

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
