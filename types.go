package main

type DiagnosticReport struct {
	Ok              bool             `json:"ok"`
	RuntimeMetadata RuntimeMetadata  `json:"runtime_metadata"`
	Diagnostics     []Diagnostic     `json:"diagnostics"`
	RawOutput       string           `json:"raw_output,omitempty"`
	Stdout          string           `json:"stdout,omitempty"`
	Stderr          string           `json:"stderr,omitempty"`
}

type RuntimeMetadata struct {
	Language        string   `json:"language,omitempty"`
	Toolchain       string   `json:"toolchain,omitempty"`
	ExecutionTimeMs int64    `json:"execution_time_ms"`
	ExitCode        int      `json:"exit_code"`
	Command         string   `json:"command,omitempty"`
	Args            []string `json:"args,omitempty"`
	Cwd             string   `json:"cwd,omitempty"`
}

type Diagnostic struct {
	Code       string             `json:"code,omitempty"`
	Severity   string             `json:"severity"`
	Message    string             `json:"message"`
	Location   *Location          `json:"location,omitempty"`
	Context    *DiagnosticContext `json:"context,omitempty"`
	RepairHint *RepairHint        `json:"repair_hint,omitempty"`
}

type Location struct {
	File   string `json:"file,omitempty"`
	Line   *int   `json:"line,omitempty"`
	Column *int   `json:"column,omitempty"`
}

type DiagnosticContext struct {
	SourceLine string `json:"source_line,omitempty"`
}

type RepairHint struct {
	Strategy string         `json:"strategy"`
	Payload  map[string]any `json:"payload"`
}
