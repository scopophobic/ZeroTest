package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultTimeoutMs = 120000
	maxOutputBytes   = 16000
)

func main() {
	s := server.NewMCPServer(
		"Diagnostic Translator",
		"0.2.0",
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
			mcp.Description("Optional language override (e.g. python, go, rust, java, cpp, typescript, ruby)"),
		),
		mcp.WithString("toolchain_hint",
			mcp.Description("Optional toolchain override (e.g. mypy, go, tsc, rustc, gcc, javac, eslint)"),
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

	diagnostics := parseDiagnostics(output, language)

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
