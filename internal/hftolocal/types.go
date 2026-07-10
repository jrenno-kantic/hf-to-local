package hftolocal

import "io"

const (
	RuntimeAll      = "all"
	RuntimeLMStudio = "lmstudio"
	RuntimeOllama   = "ollama"
	RuntimeMirror   = "mirror"

	LinkSymlink = "symlink"
	LinkHard    = "hardlink"
	LinkCopy    = "copy"

	ConflictSkip    = "skip"
	ConflictReplace = "replace"
	ConflictFail    = "fail"

	PlanText = "text"
	PlanJSON = "json"
)

var metadataNames = map[string]struct{}{
	"README.md":               {},
	"LICENSE":                 {},
	"LICENSE.txt":             {},
	"LICENSE.md":              {},
	"config.json":             {},
	"generation_config.json":  {},
	"tokenizer.json":          {},
	"tokenizer.model":         {},
	"tokenizer_config.json":   {},
	"special_tokens_map.json": {},
}

type Options struct {
	CacheDir           string
	TargetDir          string
	Runtime            string
	Repos              []string
	Revision           string
	AllSnapshots       bool
	LinkMode           string
	Overwrite          bool
	ConflictPolicy     string
	DryRun             bool
	PlanFormat         string
	IncludeMetadata    bool
	LlamaCppOnly       bool
	CreateOllama       bool
	OllamaNameTemplate string
	Stdout             io.Writer
}

type Summary struct {
	ReposSeen        int         `json:"repos_seen"`
	SnapshotsSeen    int         `json:"snapshots_seen"`
	LinksCreated     int         `json:"links_created"`
	FilesWritten     int         `json:"files_written"`
	FilesSkipped     int         `json:"files_skipped"`
	OllamaModels     int         `json:"ollama_models"`
	OllamaCreatesRun int         `json:"ollama_creates_run"`
	TotalBytes       int64       `json:"total_bytes"`
	BrokenSources    int         `json:"broken_sources"`
	Operations       []Operation `json:"operations,omitempty"`
}

type CachedRepo struct {
	RepoID   string
	CacheDir string
}

type Snapshot struct {
	Repo          CachedRepo
	RevisionLabel string
	Commit        string
	Path          string
}

type Operation struct {
	Type        string `json:"type"`
	Action      string `json:"action,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
}

type OllamaCreate struct {
	ModelName string
	ModelDir  string
}
