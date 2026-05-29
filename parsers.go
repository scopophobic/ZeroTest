package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	mypyPatternWithCol = regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s*(error|warning|note):\s*(.+?)(?:\s\[(.+)\])?$`)
	mypyPatternNoCol   = regexp.MustCompile(`^([^:]+):(\d+):\s*(error|warning|note):\s*(.+?)(?:\s\[(.+)\])?$`)
	goPattern          = regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s*(.+)$`)
	pythonFilePattern  = regexp.MustCompile(`^\s*File "([^"]+)", line (\d+)(?:, in .+)?$`)

	tsParenPattern = regexp.MustCompile(`^([^:]+)\((\d+),(\d+)\):\s*(error|warning)\s+(TS\d+):\s*(.+)$`)
	tsColonPattern = regexp.MustCompile(`^([^:]+):(\d+):(\d+)\s*[-–:]\s*(error|warning)\s+(TS\d+):\s*(.+)$`)

	eslintPattern = regexp.MustCompile(`^\s*(\d+):(\d+)\s+(error|warning)\s+(\S+)\s+(.+)$`)

	rustErrorPattern    = regexp.MustCompile(`^(error|warning)\[E(\w+)\]:\s*(.+)$`)
	rustLocationPattern = regexp.MustCompile(`^\s*-->\s*([^:]+):(\d+):(\d+)\s*$`)
	rustPanicPattern    = regexp.MustCompile(`thread\s+'[^']+'\s+panicked\s+at\s+'([^']*)',\s*([^:]+):(\d+):(\d+)`)

	cPatternWithCol = regexp.MustCompile(`^([^:]+):(\d+):(\d+):\s*(error|warning|note):\s*(.+)$`)
	cPatternNoCol   = regexp.MustCompile(`^([^:]+):(\d+):\s*(fatal\s+)?(error|warning|note):\s*(.+)$`)

	javaPattern = regexp.MustCompile(`^([^:]+):(\d+)(:\d+)?:\s*(error|warning):\s*(.+)$`)

	rubyErrorWithIn  = regexp.MustCompile(`^([^:]+):(\d+):\s*in\s+'[^']*':\s*(.+?)(?:\s+\((\w+)\))?$`)
	rubySyntaxError  = regexp.MustCompile(`^([^:]+):(\d+):\s*(syntax\s+error.*?)(?:\s+\((\w+)\))?$`)
	rubyGenericError = regexp.MustCompile(`^([^:]+):(\d+):\s*(undefined\s+method|undefined\s+local\s+variable|cannot\s+load|uninitialized\s+constant|expected|unexpected).+$`)

	eslintFilePath = regexp.MustCompile(`^(?:\.\/|\.\.\/|\/|[a-zA-Z]:\\)?[\w./\-]+\.\w+$`)
)

func parseDiagnostics(output string, language string) []Diagnostic {
	lines := splitLines(output)
	diagnostics := make([]Diagnostic, 0)

	diagnostics = append(diagnostics, parseMypyDiagnostics(lines)...)
	if pyDiag := parsePythonTraceback(lines); pyDiag != nil {
		diagnostics = append(diagnostics, *pyDiag)
	}

	if len(diagnostics) > 0 {
		return diagnostics
	}

	switch language {
	case "go":
		diagnostics = append(diagnostics, parseGoDiagnostics(lines)...)
	case "typescript":
		diagnostics = append(diagnostics, parseTypeScriptDiagnostics(lines)...)
	case "rust":
		diagnostics = append(diagnostics, parseRustDiagnostics(lines)...)
	case "c", "cpp", "c++":
		diagnostics = append(diagnostics, parseCDiagnostics(lines)...)
	case "java":
		diagnostics = append(diagnostics, parseJavaDiagnostics(lines)...)
	case "ruby":
		diagnostics = append(diagnostics, parseRubyDiagnostics(lines)...)
	case "javascript":
		diagnostics = append(diagnostics, parseESLintDiagnostics(lines)...)
		diagnostics = append(diagnostics, parseTypeScriptDiagnostics(lines)...)
	default:
		for _, p := range []func([]string) []Diagnostic{
			parseGoDiagnostics,
			parseTypeScriptDiagnostics,
			parseESLintDiagnostics,
			parseRustDiagnostics,
			parseCDiagnostics,
			parseJavaDiagnostics,
			parseRubyDiagnostics,
		} {
			diagnostics = append(diagnostics, p(lines)...)
		}
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

func parseTypeScriptDiagnostics(lines []string) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if match := tsParenPattern.FindStringSubmatch(line); match != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     fmt.Sprintf("DT_TS_%s", match[5]),
				Severity: match[4],
				Message:  match[6],
				Location: &Location{
					File:   match[1],
					Line:   intPtr(parseInt(match[2])),
					Column: intPtr(parseInt(match[3])),
				},
			})
			continue
		}
		if match := tsColonPattern.FindStringSubmatch(line); match != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     fmt.Sprintf("DT_TS_%s", match[5]),
				Severity: match[4],
				Message:  match[6],
				Location: &Location{
					File:   match[1],
					Line:   intPtr(parseInt(match[2])),
					Column: intPtr(parseInt(match[3])),
				},
			})
		}
	}
	return diagnostics
}

func parseESLintDiagnostics(lines []string) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	var currentFile string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if isLikelyFilePath(trimmed) {
			currentFile = trimmed
			continue
		}

		if match := eslintPattern.FindStringSubmatch(line); match != nil {
			d := Diagnostic{
				Code:     formatDiagnosticCode("DT_ESLINT", match[4]),
				Severity: match[3],
				Message:  match[5],
				Location: &Location{
					Line:   intPtr(parseInt(match[1])),
					Column: intPtr(parseInt(match[2])),
				},
			}
			if currentFile != "" {
				d.Location.File = currentFile
			}
			diagnostics = append(diagnostics, d)
		}
	}
	return diagnostics
}

func parseRustDiagnostics(lines []string) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if match := rustPanicPattern.FindStringSubmatch(line); match != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "DT_RS_PANIC",
				Severity: "error",
				Message:  match[1],
				Location: &Location{
					File:   match[2],
					Line:   intPtr(parseInt(match[3])),
					Column: intPtr(parseInt(match[4])),
				},
			})
		}
	}

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		match := rustErrorPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		severity := match[1]
		code := match[2]
		message := match[3]

		var file string
		var lineNum, colNum int
		for j := i + 1; j < len(lines); j++ {
			nextLine := strings.TrimSpace(lines[j])
			if nextLine == "" {
				continue
			}
			if locMatch := rustLocationPattern.FindStringSubmatch(nextLine); locMatch != nil {
				file = locMatch[1]
				lineNum = parseInt(locMatch[2])
				colNum = parseInt(locMatch[3])
				break
			}
			if rustErrorPattern.MatchString(nextLine) {
				break
			}
		}

		d := Diagnostic{
			Code:     fmt.Sprintf("DT_RS_E%s", code),
			Severity: severity,
			Message:  message,
		}
		if file != "" {
			d.Location = &Location{
				File:   file,
				Line:   intPtr(lineNum),
				Column: intPtr(colNum),
			}
		}
		diagnostics = append(diagnostics, d)
	}

	return diagnostics
}

func parseCDiagnostics(lines []string) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if match := cPatternWithCol.FindStringSubmatch(line); match != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     formatDiagnosticCode("DT_C", match[4]),
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
		if match := cPatternNoCol.FindStringSubmatch(line); match != nil {
			severity := match[4]
			if match[3] != "" {
				severity = "fatal " + severity
			}
			diagnostics = append(diagnostics, Diagnostic{
				Code:     formatDiagnosticCode("DT_C", strings.ReplaceAll(severity, " ", "_")),
				Severity: severity,
				Message:  match[5],
				Location: &Location{
					File: match[1],
					Line: intPtr(parseInt(match[2])),
				},
			})
		}
	}
	return diagnostics
}

func parseJavaDiagnostics(lines []string) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if match := javaPattern.FindStringSubmatch(line); match != nil {
			d := Diagnostic{
				Code:     formatDiagnosticCode("DT_JV", match[4]),
				Severity: match[4],
				Message:  match[5],
				Location: &Location{
					File: match[1],
					Line: intPtr(parseInt(match[2])),
				},
			}
			colStr := strings.TrimPrefix(match[3], ":")
			if colStr != "" {
				d.Location.Column = intPtr(parseInt(colStr))
			}
			diagnostics = append(diagnostics, d)
		}
	}
	return diagnostics
}

func parseRubyDiagnostics(lines []string) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if match := rubySyntaxError.FindStringSubmatch(line); match != nil {
			d := Diagnostic{
				Code:     "DT_RB_GENERIC",
				Severity: "error",
				Message:  match[3],
				Location: &Location{
					File: match[1],
					Line: intPtr(parseInt(match[2])),
				},
			}
			if match[4] != "" {
				d.Code = formatDiagnosticCode("DT_RB", match[4])
			}
			diagnostics = append(diagnostics, d)
			continue
		}

		if match := rubyErrorWithIn.FindStringSubmatch(line); match != nil {
			d := Diagnostic{
				Code:     "DT_RB_GENERIC",
				Severity: "error",
				Message:  match[3],
				Location: &Location{
					File: match[1],
					Line: intPtr(parseInt(match[2])),
				},
			}
			if match[4] != "" {
				d.Code = formatDiagnosticCode("DT_RB", match[4])
			}
			diagnostics = append(diagnostics, d)
			continue
		}

		if match := rubyGenericError.FindStringSubmatch(line); match != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "DT_RB_GENERIC",
				Severity: "error",
				Message:  match[0],
				Location: &Location{
					File: match[1],
					Line: intPtr(parseInt(match[2])),
				},
			})
		}
	}
	return diagnostics
}

func detectLanguage(command string, output string) string {
	base := strings.ToLower(filepath.Base(command))
	switch {
	case strings.Contains(base, "python"), strings.Contains(base, "mypy"),
		strings.Contains(base, "pytest"), strings.Contains(base, "flake8"),
		strings.Contains(base, "black"), strings.Contains(base, "pylint"):
		return "python"
	case strings.Contains(base, "golangci"), strings.Contains(base, "golang"):
		return "go"
	case strings.Contains(base, "cargo"), strings.Contains(base, "rustc"),
		strings.Contains(base, "rustup"):
		return "rust"
	case strings.Contains(base, "go"):
		return "go"
	case strings.Contains(base, "node"), strings.Contains(base, "npm"),
		strings.Contains(base, "yarn"), strings.Contains(base, "npx"),
		strings.Contains(base, "pnpm"), strings.Contains(base, "bun"):
		return "javascript"
	case strings.Contains(base, "tsc"), strings.Contains(base, "typescript"),
		strings.Contains(base, "ts-node"), strings.Contains(base, "deno"):
		return "typescript"
	case strings.Contains(base, "eslint"), strings.Contains(base, "prettier"):
		return "javascript"
	case strings.Contains(base, "gcc"), strings.Contains(base, "g++"),
		strings.Contains(base, "clang"), strings.Contains(base, "cc"),
		strings.Contains(base, "clang++"), strings.Contains(base, "gccgo"):
		return "c"
	case strings.Contains(base, "javac"), strings.Contains(base, "java"),
		strings.Contains(base, "mvn"), strings.Contains(base, "gradle"),
		strings.Contains(base, "maven"):
		return "java"
	case strings.Contains(base, "ruby"), strings.Contains(base, "rake"),
		strings.Contains(base, "rails"), strings.Contains(base, "bundler"),
		strings.Contains(base, "gem"):
		return "ruby"
	case strings.Contains(base, "swift"):
		return "swift"
	}
	if strings.Contains(output, "Traceback (most recent call last):") {
		return "python"
	}
	if strings.Contains(output, "thread '") && strings.Contains(output, "panicked") {
		return "rust"
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
	case strings.Contains(base, "flake8"):
		return "flake8"
	case strings.Contains(base, "black"):
		return "black"
	case strings.Contains(base, "pylint"):
		return "pylint"
	case strings.Contains(base, "python"):
		return "python"
	case strings.Contains(base, "golangci"), strings.Contains(base, "golang"):
		return "go"
	case strings.Contains(base, "cargo"):
		return "cargo"
	case strings.Contains(base, "go"):
		return "go"
	case strings.Contains(base, "tsc"), strings.Contains(base, "typescript"):
		return "tsc"
	case strings.Contains(base, "eslint"):
		return "eslint"
	case strings.Contains(base, "prettier"):
		return "prettier"
	case strings.Contains(base, "node"), strings.Contains(base, "npm"),
		strings.Contains(base, "npx"), strings.Contains(base, "yarn"),
		strings.Contains(base, "pnpm"):
		return "node"
	case strings.Contains(base, "rustc"):
		return "rustc"
	case strings.Contains(base, "cargo"):
		return "cargo"
	case strings.Contains(base, "gcc"), strings.Contains(base, "g++"):
		return "gcc"
	case strings.Contains(base, "clang"), strings.Contains(base, "clang++"):
		return "clang"
	case strings.Contains(base, "javac"):
		return "javac"
	case strings.Contains(base, "mvn"), strings.Contains(base, "maven"):
		return "maven"
	case strings.Contains(base, "gradle"):
		return "gradle"
	case strings.Contains(base, "ruby"):
		return "ruby"
	case strings.Contains(base, "rake"):
		return "rake"
	case strings.Contains(base, "rails"):
		return "rails"
	case strings.Contains(base, "swift"):
		return "swift"
	case strings.Contains(output, "Traceback (most recent call last):"):
		return "python"
	case strings.Contains(output, "error["):
		return "rustc"
	}
	return "unknown"
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

func isLikelyFilePath(s string) bool {
	return eslintFilePath.MatchString(s) && !strings.Contains(s, " ") && !strings.HasPrefix(s, "-")
}
