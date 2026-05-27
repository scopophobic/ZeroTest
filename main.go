package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultTimeoutMs = 120000
	maxOutputBytes   = 16000
)

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

var (
	mypyPatternWithCol = regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s*(error|warning|note):\s*(.+?)(?:\s\[(.+)\])?$`)
	mypyPatternNoCol   = regexp.MustCompile(`^([^:]+):(\d+):\s*(error|warning|note):\s*(.+?)(?:\s\[(.+)\])?$`)
	goPattern          = regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s*(.+)$`)
	pythonFilePattern  = regexp.MustCompile(`^\s*File "([^"]+)", line (\d+)(?:, in .+)?$`)
)

func main() {
	s := server.NewMCPServer(
		"Diagnostic Translator",
		"0.1.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	tool := mcp.NewTool("diagnose_command",
		mcp.WithDescription("Run a command and return structured diagnostics from its output"),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Executable to run, for example: python"),
		),
		mcp.WithArray("args",
			mcp.Description("Arguments to pass to the command"),
			mcp.WithStringItems(),
		),
		mcp.WithString("cwd",
			mcp.Description("Working directory for the command"),
		),
		mcp.WithInteger("timeout_ms",
			mcp.Description("Timeout in milliseconds"),
		),
		mcp.WithString("language_hint",
			mcp.Description("Optional language override (e.g. python, go)"),
		),
		mcp.WithString("toolchain_hint",
			mcp.Description("Optional toolchain override (e.g. mypy, go)"),
		),
	)

	s.AddTool(tool, diagnoseHandler)

	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func diagnoseHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	command, err := request.RequireString("command")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	args := request.GetStringSlice("args", nil)
	if len(args) == 0 && strings.Contains(command, " ") {
		fields := strings.Fields(command)
		command = fields[0]
		args = fields[1:]
	}

	cwd := request.GetString("cwd", "")
	timeoutMs := request.GetInt("timeout_ms", defaultTimeoutMs)
	languageHint := strings.TrimSpace(request.GetString("language_hint", ""))
	toolchainHint := strings.TrimSpace(request.GetString("toolchain_hint", ""))

	report := runAndDiagnose(ctx, command, args, cwd, timeoutMs, languageHint, toolchainHint)
	return mcp.NewToolResultStructuredOnly(report), nil
}

func runAndDiagnose(ctx context.Context, command string, args []string, cwd string, timeoutMs int, languageHint string, toolchainHint string) DiagnosticReport {
	start := time.Now()

	execCtx := ctx
	var cancel context.CancelFunc
	if timeoutMs > 0 {
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}

	cmd := exec.CommandContext(execCtx, command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	execMs := time.Since(start).Milliseconds()

	exitCode := 0
	if runErr != nil {
		if exitErr := new(exec.ExitError); errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(runErr, context.DeadlineExceeded) {
			exitCode = -1
		} else {
			exitCode = 1
		}
	}

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	output := stderrStr
	if strings.TrimSpace(output) == "" {
		output = stdoutStr
	}

	language := languageHint
	if language == "" {
		language = detectLanguage(command, output)
	}

	toolchain := toolchainHint
	if toolchain == "" {
		toolchain = detectToolchain(command, output)
	}

	diagnostics := parseDiagnostics(output)

	if errors.Is(runErr, context.DeadlineExceeded) {
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "DT_TIMEOUT",
			Severity: "error",
			Message:  "Command timed out before completion",
		})
	}

	if runErr != nil && len(diagnostics) == 0 {
		message := strings.TrimSpace(output)
		if message == "" {
			message = runErr.Error()
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "DT_GENERIC",
			Severity: "error",
			Message:  message,
		})
	}

	return DiagnosticReport{
		Ok: runErr == nil,
		RuntimeMetadata: RuntimeMetadata{
			Language:        language,
			Toolchain:       toolchain,
			ExecutionTimeMs: execMs,
			ExitCode:        exitCode,
			Command:         command,
			Args:            args,
			Cwd:             cwd,
		},
		Diagnostics: diagnostics,
		RawOutput:   truncateOutput(output, maxOutputBytes),
		Stdout:      truncateOutput(stdoutStr, maxOutputBytes),
		Stderr:      truncateOutput(stderrStr, maxOutputBytes),
	}
}

func parseDiagnostics(output string) []Diagnostic {
	lines := splitLines(output)
	diagnostics := make([]Diagnostic, 0)

	mypyDiagnostics := parseMypyDiagnostics(lines)
	diagnostics = append(diagnostics, mypyDiagnostics...)

	if pyDiagnostic := parsePythonTraceback(lines); pyDiagnostic != nil {
		diagnostics = append(diagnostics, *pyDiagnostic)
	}

	if len(mypyDiagnostics) == 0 {
		diagnostics = append(diagnostics, parseGoDiagnostics(lines)...)
	}

	if len(diagnostics) == 0 {
		message := strings.TrimSpace(lastNonEmptyLine(lines))
		if message != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "DT_GENERIC",
				Severity: "error",
				Message:  message,
			})
		}
	}

	return diagnostics
}

func parseMypyDiagnostics(lines []string) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if match := mypyPatternWithCol.FindStringSubmatch(line); match != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     formatDiagnosticCode("DT_PY", match[6]),
				Severity: match[4],
				Message:  match[5],
				Location: &Location{
					File:   match[1],
					Line:   intPtr(parseInt(match[2])),
					Column: intPtr(parseInt(match[3])),
				},
			})
			continue
		}

		if match := mypyPatternNoCol.FindStringSubmatch(line); match != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     formatDiagnosticCode("DT_PY", match[5]),
				Severity: match[3],
				Message:  match[4],
				Location: &Location{
					File: match[1],
					Line: intPtr(parseInt(match[2])),
				},
			})
		}
	}
	return diagnostics
}

func parsePythonTraceback(lines []string) *Diagnostic {
	var file string
	var lineNum int
	var sourceLine string

	for i := 0; i < len(lines); i++ {
		match := pythonFilePattern.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}

		file = match[1]
		lineNum = parseInt(match[2])
		if i+1 < len(lines) {
			trimmed := strings.TrimSpace(lines[i+1])
			if trimmed != "" && !strings.HasPrefix(trimmed, "File ") {
				sourceLine = trimmed
			}
		}
	}

	exType, exMessage := parsePythonException(lines)
	if exType == "" && exMessage == "" {
		return nil
	}

	message := exMessage
	if message == "" {
		message = exType
	}

	diagnostic := Diagnostic{
		Code:     "DT_PY_RUNTIME",
		Severity: "error",
		Message:  message,
	}

	if file != "" {
		diagnostic.Location = &Location{
			File:   file,
			Line:   intPtr(lineNum),
			Column: intPtr(1),
		}
	}

	if sourceLine != "" {
		diagnostic.Context = &DiagnosticContext{
			SourceLine: sourceLine,
		}
	}

	return &diagnostic
}

func parseGoDiagnostics(lines []string) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := goPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "DT_GO_BUILD",
			Severity: "error",
			Message:  match[4],
			Location: &Location{
				File:   match[1],
				Line:   intPtr(parseInt(match[2])),
				Column: intPtr(parseInt(match[3])),
			},
		})
	}
	return diagnostics
}

func detectLanguage(command string, output string) string {
	base := strings.ToLower(filepath.Base(command))
	if strings.Contains(base, "python") || strings.Contains(base, "mypy") || strings.Contains(base, "pytest") {
		return "python"
	}
	if strings.Contains(base, "go") || strings.Contains(base, "golang") {
		return "go"
	}
	if strings.Contains(base, "node") || strings.Contains(base, "npm") || strings.Contains(base, "yarn") {
		return "javascript"
	}
	if strings.Contains(output, "Traceback (most recent call last):") {
		return "python"
	}
	return "unknown"
}

func detectToolchain(command string, output string) string {
	base := strings.ToLower(filepath.Base(command))
	switch {
	case strings.Contains(base, "mypy"):
		return "mypy"
	case strings.Contains(base, "pytest"):
		return "pytest"
	case strings.Contains(base, "python"):
		return "python"
	case strings.Contains(base, "go"):
		return "go"
	case strings.Contains(output, "Traceback (most recent call last):"):
		return "python"
	default:
		return "unknown"
	}
}

func splitLines(output string) []string {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	return strings.Split(output, "\n")
}

func lastNonEmptyLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func parsePythonException(lines []string) (string, string) {
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Traceback") {
			break
		}
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			exType := strings.TrimSpace(parts[0])
			exMessage := strings.TrimSpace(parts[1])
			if isPythonException(exType) {
				return exType, exMessage
			}
		} else if isPythonException(line) {
			return line, ""
		}
	}
	return "", ""
}

func isPythonException(name string) bool {
	return strings.HasSuffix(name, "Error") || strings.HasSuffix(name, "Exception") || strings.HasSuffix(name, "Warning")
}

func formatDiagnosticCode(prefix string, code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Sprintf("%s_GENERIC", prefix)
	}
	code = strings.ReplaceAll(code, "-", "_")
	code = strings.ReplaceAll(code, " ", "_")
	return fmt.Sprintf("%s_%s", prefix, strings.ToUpper(code))
}

func parseInt(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func intPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func truncateOutput(output string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(output) <= maxBytes {
		return output
	}
	trimmed := output[:maxBytes]
	return strings.TrimRight(trimmed, "\n") + "\n[output truncated]"
}
