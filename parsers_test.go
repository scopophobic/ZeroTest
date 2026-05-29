package main

import (
	"fmt"
	"strings"
	"testing"
)

func eqLoc(t *testing.T, got, want *Location) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil || want == nil {
		t.Errorf("location: got %+v, want %+v", got, want)
		return
	}
	if got.File != want.File {
		t.Errorf("location.File: got %q, want %q", got.File, want.File)
	}
	if got.Line != nil && want.Line != nil && *got.Line != *want.Line {
		t.Errorf("location.Line: got %d, want %d", *got.Line, *want.Line)
	}
	if got.Column != nil && want.Column != nil && *got.Column != *want.Column {
		t.Errorf("location.Column: got %d, want %d", *got.Column, *want.Column)
	}
}

func diagSummary(d Diagnostic) string {
	return fmt.Sprintf("code=%s sev=%s msg=%q file=%s line=%d col=%d",
		d.Code, d.Severity, d.Message,
		func() string { if d.Location != nil { return d.Location.File }; return "" }(),
		func() int { if d.Location != nil && d.Location.Line != nil { return *d.Location.Line }; return 0 }(),
		func() int { if d.Location != nil && d.Location.Column != nil { return *d.Location.Column }; return 0 }(),
	)
}

func diagSummaries(ds []Diagnostic) string {
	var b strings.Builder
	for i, d := range ds {
		if i > 0 { b.WriteString("; ") }
		b.WriteString(diagSummary(d))
	}
	return b.String()
}

func TestParseMypy(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		count  int
		verify func(*testing.T, []Diagnostic)
	}{
		{
			name:  "with column and error code",
			input: `src/app.py:42:18: error: Unsupported operand types for +: "int" and "str"  [operator]`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_PY_OPERATOR" { t.Errorf("code=%s", d[0].Code) }
				if d[0].Severity != "error" { t.Errorf("severity=%s", d[0].Severity) }
				if d[0].Location == nil { t.Fatal("nil location") }
				if d[0].Location.File != "src/app.py" { t.Errorf("file=%s", d[0].Location.File) }
				if *d[0].Location.Line != 42 { t.Errorf("line=%d", *d[0].Location.Line) }
				if *d[0].Location.Column != 18 { t.Errorf("col=%d", *d[0].Location.Column) }
			},
		},
		{
			name:  "without column",
			input: `src/app.py:42: error: "int" is not subscriptable  [misc]`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_PY_MISC" { t.Errorf("code=%s", d[0].Code) }
				if d[0].Location.Column != nil { t.Errorf("expected nil column") }
				if *d[0].Location.Line != 42 { t.Errorf("line=%d", *d[0].Location.Line) }
			},
		},
		{
			name:  "warning level",
			input: `src/app.py:10:5: warning: Unused variable "x"  [unused]`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_PY_UNUSED" { t.Errorf("code=%s", d[0].Code) }
				if d[0].Severity != "warning" { t.Errorf("severity=%s", d[0].Severity) }
			},
		},
		{
			name:  "note level",
			input: `src/app.py:10:5: note: See https://mypy.readthedocs.io/  [misc]`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Severity != "note" { t.Errorf("severity=%s", d[0].Severity) }
			},
		},
		{
			name:  "multiple diagnostics",
			input:  "src/a.py:1: error: bad  [err1]\nsrc/b.py:2:5: note: info  [err2]",
			count: 2,
		},
		{
			name:  "no code suffix",
			input: `src/app.py:42:18: error: Need type annotation`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_PY_GENERIC" { t.Errorf("code=%s", d[0].Code) }
			},
		},
		{
			name:  "no match",
			input: `this is just a random line`,
			count: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMypyDiagnostics(splitLines(tc.input))
			if len(got) != tc.count {
				t.Errorf("got %d diagnostics, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
			if tc.verify != nil && len(got) > 0 {
				tc.verify(t, got)
			}
		})
	}
}

func TestParsePythonTraceback(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   *Diagnostic
	}{
		{
			name:  "full traceback",
			input: "Traceback (most recent call last):\n  File \"app.py\", line 10, in main\n    result = 1 / 0\n  File \"app.py\", line 15, in <module>\n    main()\nZeroDivisionError: division by zero",
			want: &Diagnostic{
				Code: "DT_PY_RUNTIME", Severity: "error", Message: "division by zero",
				Location: &Location{File: "app.py", Line: intPtr(15), Column: intPtr(1)},
				Context:  &DiagnosticContext{SourceLine: "main()"},
			},
		},
		{
			name:   "no traceback",
			input:  "hello world",
			want:   nil,
		},
		{
			name:  "key error",
			input: "Traceback (most recent call last):\n  File \"app.py\", line 5, in <module>\n    d['missing']\nKeyError: 'missing'",
			want: &Diagnostic{
				Code: "DT_PY_RUNTIME", Severity: "error", Message: "'missing'",
				Location: &Location{File: "app.py", Line: intPtr(5), Column: intPtr(1)},
				Context:  &DiagnosticContext{SourceLine: "d['missing']"},
			},
		},
		{
			name:  "import error",
			input: "Traceback (most recent call last):\n  File \"app.py\", line 1, in <module>\n    import nonexistent\nModuleNotFoundError: No module named 'nonexistent'",
			want: &Diagnostic{
				Code: "DT_PY_RUNTIME", Severity: "error", Message: "No module named 'nonexistent'",
				Location: &Location{File: "app.py", Line: intPtr(1), Column: intPtr(1)},
				Context:  &DiagnosticContext{SourceLine: "import nonexistent"},
			},
		},
		{
			name:  "exception only no line info",
			input: "ValueError: invalid literal for int() with base 10: 'abc'",
			want: &Diagnostic{
				Code: "DT_PY_RUNTIME", Severity: "error", Message: "invalid literal for int() with base 10: 'abc'",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePythonTraceback(splitLines(tc.input))
			if tc.want == nil {
				if got != nil { t.Errorf("expected nil, got %+v", got) }
				return
			}
			if got == nil { t.Fatal("expected diagnostic, got nil") }
			if got.Code != tc.want.Code { t.Errorf("code=%s", got.Code) }
			if got.Severity != tc.want.Severity { t.Errorf("severity=%s", got.Severity) }
			if got.Message != tc.want.Message { t.Errorf("message=%q", got.Message) }
			eqLoc(t, got.Location, tc.want.Location)
			if tc.want.Context != nil {
				if got.Context == nil { t.Errorf("expected context, got nil") }
			}
		})
	}
}

func TestParseGo(t *testing.T) {
	tests := []struct {
		name  string
		input string
		count int
	}{
		{
			name:  "go build error",
			input: "main.go:42:18: undefined: foo",
			count: 1,
		},
		{
			name:  "go multiple errors",
			input: "main.go:10:2: cannot use a (type int) as type string in assignment\nutil.go:5:9: undefined: helper",
			count: 2,
		},
		{
			name:  "go package prefix line skipped",
			input: "# package/path\nmain.go:10:2: some error",
			count: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseGoDiagnostics(splitLines(tc.input))
			if len(got) != tc.count {
				t.Errorf("got %d, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
		})
	}
}

func TestParseTypeScript(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		count  int
		verify func(*testing.T, []Diagnostic)
	}{
		{
			name:  "paren format",
			input: `src/app.ts(5,2): error TS2322: Type 'number' is not assignable to type 'string'.`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_TS_TS2322" { t.Errorf("code=%s", d[0].Code) }
				if *d[0].Location.Line != 5 { t.Errorf("line=%d", *d[0].Location.Line) }
				if *d[0].Location.Column != 2 { t.Errorf("col=%d", *d[0].Location.Column) }
			},
		},
		{
			name:  "colon format",
			input: `src/app.ts:10:15 - error TS2554: Expected 2 arguments, but got 1.`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_TS_TS2554" { t.Errorf("code=%s", d[0].Code) }
				if *d[0].Location.Line != 10 { t.Errorf("line=%d", *d[0].Location.Line) }
			},
		},
		{
			name:  "warning",
			input: `src/app.ts:3:7 - warning TS6133: 'x' is declared but its value is never read.`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Severity != "warning" { t.Errorf("severity=%s", d[0].Severity) }
			},
		},
		{
			name:  "multiple errors",
			input:  "src/a.ts(1,1): error TS1000: err1\nsrc/b.ts:2:3 - warning TS2000: warn2",
			count: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTypeScriptDiagnostics(splitLines(tc.input))
			if len(got) != tc.count {
				t.Errorf("got %d, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
			if tc.verify != nil && len(got) > 0 {
				tc.verify(t, got)
			}
		})
	}
}

func TestParseESLint(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		count  int
		verify func(*testing.T, []Diagnostic)
	}{
		{
			name:  "basic eslint",
			input: "src/app.ts\n  42:18  error  no-unused-vars  'foo' is defined but never used",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_ESLINT_NO_UNUSED_VARS" { t.Errorf("code=%s", d[0].Code) }
				if d[0].Severity != "error" { t.Errorf("severity=%s", d[0].Severity) }
				if d[0].Location.File != "src/app.ts" { t.Errorf("file=%s", d[0].Location.File) }
				if *d[0].Location.Line != 42 { t.Errorf("line=%d", *d[0].Location.Line) }
				if *d[0].Location.Column != 18 { t.Errorf("col=%d", *d[0].Location.Column) }
			},
		},
		{
			name:  "warning level",
			input: "src/util.js\n  5:1  warning  no-console  Unexpected console statement.",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Severity != "warning" { t.Errorf("severity=%s", d[0].Severity) }
			},
		},
		{
			name:  "multiple files",
			input: "src/a.ts\n  1:1  error  rule1  msg1\n\nsrc/b.ts\n  2:2  warning  rule2  msg2",
			count: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseESLintDiagnostics(splitLines(tc.input))
			if len(got) != tc.count {
				t.Errorf("got %d, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
			if tc.verify != nil && len(got) > 0 {
				tc.verify(t, got)
			}
		})
	}
}

func TestParseRust(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		count  int
		verify func(*testing.T, []Diagnostic)
	}{
		{
			name:  "compile error with location",
			input: "error[E0308]: mismatched types\n --> src/main.rs:42:18",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_RS_E0308" { t.Errorf("code=%s", d[0].Code) }
				if *d[0].Location.Line != 42 { t.Errorf("line=%d", *d[0].Location.Line) }
				if *d[0].Location.Column != 18 { t.Errorf("col=%d", *d[0].Location.Column) }
			},
		},
		{
			name:  "panic",
			input: `thread 'main' panicked at 'index out of bounds: the len is 3 but the index is 5', src/main.rs:10:18`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_RS_PANIC" { t.Errorf("code=%s", d[0].Code) }
				if *d[0].Location.Line != 10 { t.Errorf("line=%d", *d[0].Location.Line) }
				if *d[0].Location.Column != 18 { t.Errorf("col=%d", *d[0].Location.Column) }
			},
		},
		{
			name:  "warning",
			input: "warning[E0001]: unused variable `x`",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Severity != "warning" { t.Errorf("severity=%s", d[0].Severity) }
				if d[0].Location != nil { t.Errorf("expected nil location for no --> line") }
			},
		},
		{
			name:  "multiple errors",
			input: "error[E0308]: type mismatch\n --> src/a.rs:1:2\nerror[E0432]: unresolved import\n --> src/b.rs:3:5",
			count: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRustDiagnostics(splitLines(tc.input))
			if len(got) != tc.count {
				t.Errorf("got %d, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
			if tc.verify != nil && len(got) > 0 {
				tc.verify(t, got)
			}
		})
	}
}

func TestParseC(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		count  int
		verify func(*testing.T, []Diagnostic)
	}{
		{
			name:  "gcc error with column",
			input: `main.c:42:18: error: 'foo' undeclared (first use in this function)`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_C_ERROR" { t.Errorf("code=%s", d[0].Code) }
				if *d[0].Location.Line != 42 { t.Errorf("line=%d", *d[0].Location.Line) }
				if *d[0].Location.Column != 18 { t.Errorf("col=%d", *d[0].Location.Column) }
			},
		},
		{
			name:  "gcc fatal error no column",
			input: `main.c:5: fatal error: stdio.h: No such file or directory`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_C_FATAL_ERROR" { t.Errorf("code=%s", d[0].Code) }
				if d[0].Severity != "fatal error" { t.Errorf("severity=%s", d[0].Severity) }
				if d[0].Location.Column != nil { t.Errorf("expected nil column") }
			},
		},
		{
			name:  "clang warning",
			input: `test.cpp:10:5: warning: implicit declaration of function 'bar'`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Severity != "warning" { t.Errorf("severity=%s", d[0].Severity) }
			},
		},
		{
			name:  "note",
			input: `test.c:1:1: note: expected 'int *' but argument is of type 'float *'`,
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Severity != "note" { t.Errorf("severity=%s", d[0].Severity) }
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCDiagnostics(splitLines(tc.input))
			if len(got) != tc.count {
				t.Errorf("got %d, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
			if tc.verify != nil && len(got) > 0 {
				tc.verify(t, got)
			}
		})
	}
}

func TestParseJava(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		count  int
		verify func(*testing.T, []Diagnostic)
	}{
		{
			name:  "javac error without column",
			input: "Test.java:5: error: ';' expected",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_JV_ERROR" { t.Errorf("code=%s", d[0].Code) }
				if *d[0].Location.Line != 5 { t.Errorf("line=%d", *d[0].Location.Line) }
				if d[0].Location.Column != nil { t.Errorf("expected nil column") }
			},
		},
		{
			name:  "javac error with column",
			input: "Test.java:5:18: error: cannot find symbol",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if *d[0].Location.Line != 5 { t.Errorf("line=%d", *d[0].Location.Line) }
				if d[0].Location.Column != nil && *d[0].Location.Column != 18 {
					t.Errorf("col=%d", *d[0].Location.Column)
				}
			},
		},
		{
			name:  "warning",
			input: "Main.java:3: warning: [unchecked] unchecked cast",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Severity != "warning" { t.Errorf("severity=%s", d[0].Severity) }
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseJavaDiagnostics(splitLines(tc.input))
			if len(got) != tc.count {
				t.Errorf("got %d, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
			if tc.verify != nil && len(got) > 0 {
				tc.verify(t, got)
			}
		})
	}
}

func TestParseRuby(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		count  int
		verify func(*testing.T, []Diagnostic)
	}{
		{
			name:  "syntax error",
			input: "test.rb:5: syntax error, unexpected end-of-input (SyntaxError)",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_RB_SYNTAXERROR" { t.Errorf("code=%s", d[0].Code) }
				if *d[0].Location.Line != 5 { t.Errorf("line=%d", *d[0].Location.Line) }
			},
		},
		{
			name:  "method error",
			input: "test.rb:10:in 'foo': undefined method 'bar' for nil:NilClass (NoMethodError)",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_RB_NOMETHODERROR" { t.Errorf("code=%s", d[0].Code) }
			},
		},
		{
			name:  "undefined method inline",
			input: "app.rb:3: undefined method `foo' for main:Object (NoMethodError)",
			count: 1,
			verify: func(t *testing.T, d []Diagnostic) {
				if d[0].Code != "DT_RB_GENERIC" { t.Errorf("code=%s", d[0].Code) }
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRubyDiagnostics(splitLines(tc.input))
			if len(got) != tc.count {
				t.Errorf("got %d, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
			if tc.verify != nil && len(got) > 0 {
				tc.verify(t, got)
			}
		})
	}
}

func TestParseDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		language string
		count    int
	}{
		{
			name:     "mypy takes priority",
			output:   "src/app.py:42:18: error: bad type  [operator]\nsome random line",
			language: "python",
			count:    1,
		},
		{
			name:     "go via language dispatch",
			output:   "main.go:10:2: undefined: helper",
			language: "go",
			count:    1,
		},
		{
			name:     "ts via language dispatch",
			output:   "src/app.ts(5,2): error TS2322: type mismatch",
			language: "typescript",
			count:    1,
		},
		{
			name:     "unknown language tries all",
			output:   "src/app.ts(5,2): error TS2322: type mismatch",
			language: "unknown",
			count:    1,
		},
		{
			name:     "python traceback in unknown",
			output:   "Traceback (most recent call last):\n  File \"app.py\", line 1, in <module>\nKeyError: 'x'",
			language: "unknown",
			count:    1,
		},
		{
			name:     "generic fallback",
			output:   "something went wrong\ncommand not found\nBuild failed",
			language: "unknown",
			count:    1,
		},
		{
			name:     "empty output",
			output:   "",
			language: "unknown",
			count:    0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDiagnostics(tc.output, tc.language)
			if len(got) != tc.count {
				t.Errorf("got %d diagnostics, want %d\n%s", len(got), tc.count, diagSummaries(got))
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		command string
		output  string
		want    string
	}{
		{command: "python3", want: "python"},
		{command: "mypy", want: "python"},
		{command: "pytest", want: "python"},
		{command: "flake8", want: "python"},
		{command: "pylint", want: "python"},
		{command: "go", want: "go"},
		{command: "golangci-lint", want: "go"},
		{command: "tsc", want: "typescript"},
		{command: "eslint", want: "javascript"},
		{command: "node", want: "javascript"},
		{command: "npm", want: "javascript"},
		{command: "rustc", want: "rust"},
		{command: "cargo", want: "rust"},
		{command: "gcc", want: "c"},
		{command: "g++", want: "c"},
		{command: "clang", want: "c"},
		{command: "clang++", want: "c"},
		{command: "javac", want: "java"},
		{command: "mvn", want: "java"},
		{command: "gradle", want: "java"},
		{command: "ruby", want: "ruby"},
		{command: "rake", want: "ruby"},
		{command: "rails", want: "ruby"},
		{command: "swift", want: "swift"},
		{command: "unknown_tool", output: "Traceback (most recent call last):\n  File \"x.py\"\nKeyError", want: "python"},
		{command: "unknown_tool", output: "thread '<main>' has panicked at 'test'", want: "rust"},
		{command: "unknown_tool", output: "just some text", want: "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.command+"/"+tc.want, func(t *testing.T) {
			got := detectLanguage(tc.command, tc.output)
			if got != tc.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tc.command, got, tc.want)
			}
		})
	}
}

func TestDetectToolchain(t *testing.T) {
	tests := []struct {
		command string
		output  string
		want    string
	}{
		{command: "mypy", want: "mypy"},
		{command: "python3", want: "python"},
		{command: "go", want: "go"},
		{command: "tsc", want: "tsc"},
		{command: "eslint", want: "eslint"},
		{command: "rustc", want: "rustc"},
		{command: "cargo", want: "cargo"},
		{command: "gcc", want: "gcc"},
		{command: "g++", want: "gcc"},
		{command: "clang", want: "clang"},
		{command: "javac", want: "javac"},
		{command: "mvn", want: "maven"},
		{command: "gradle", want: "gradle"},
		{command: "ruby", want: "ruby"},
		{command: "swift", want: "swift"},
		{command: "unknown_tool", output: "Traceback (most recent call last):", want: "python"},
		{command: "unknown_tool", output: "error[E0308]", want: "rustc"},
		{command: "npx", want: "node"},
		{command: "yarn", want: "node"},
		{command: "unknown", want: "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.command+"/"+tc.want, func(t *testing.T) {
			got := detectToolchain(tc.command, tc.output)
			if got != tc.want {
				t.Errorf("detectToolchain(%q) = %q, want %q", tc.command, got, tc.want)
			}
		})
	}
}

func TestFormatDiagnosticCode(t *testing.T) {
	tests := []struct {
		prefix string
		code   string
		want   string
	}{
		{prefix: "DT_PY", code: "operator", want: "DT_PY_OPERATOR"},
		{prefix: "DT_PY", code: "", want: "DT_PY_GENERIC"},
		{prefix: "DT_PY", code: "arg-type", want: "DT_PY_ARG_TYPE"},
		{prefix: "DT_TS", code: "TS2322", want: "DT_TS_TS2322"},
		{prefix: "DT_C", code: "error", want: "DT_C_ERROR"},
		{prefix: "DT_JV", code: "warning", want: "DT_JV_WARNING"},
	}
	for _, tc := range tests {
		got := formatDiagnosticCode(tc.prefix, tc.code)
		if got != tc.want {
			t.Errorf("formatDiagnosticCode(%q, %q) = %q, want %q", tc.prefix, tc.code, got, tc.want)
		}
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		maxBytes int
		want     string
	}{
		{
			name:     "short output",
			output:   "hello",
			maxBytes: 100,
			want:     "hello",
		},
		{
			name:     "truncated",
			output:   "hello world",
			maxBytes: 5,
			want:     "hello\n[output truncated]",
		},
		{
			name:     "zero max",
			output:   "hello",
			maxBytes: 0,
			want:     "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateOutput(tc.output, tc.maxBytes)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIntPtr(t *testing.T) {
	if v := intPtr(0); v != nil { t.Errorf("expected nil for 0") }
	if v := intPtr(5); v == nil || *v != 5 { t.Errorf("expected 5, got %v", v) }
}

func TestParseInt(t *testing.T) {
	if v := parseInt("42"); v != 42 { t.Errorf("got %d", v) }
	if v := parseInt("abc"); v != 0 { t.Errorf("got %d", v) }
	if v := parseInt(""); v != 0 { t.Errorf("got %d", v) }
}

func TestSplitLines(t *testing.T) {
	if got := splitLines("a\nb\nc"); len(got) != 3 { t.Errorf("got %d lines", len(got)) }
	if got := splitLines("a\r\nb\r\nc"); len(got) != 3 { t.Errorf("got %d lines", len(got)) }
	if got := splitLines(""); len(got) != 1 || got[0] != "" { t.Errorf("got %v", got) }
}

func TestLastNonEmptyLine(t *testing.T) {
	if v := lastNonEmptyLine([]string{"a", "b", "c"}); v != "c" { t.Errorf("got %q", v) }
	if v := lastNonEmptyLine([]string{"a", "", ""}); v != "a" { t.Errorf("got %q", v) }
	if v := lastNonEmptyLine([]string{"", "", ""}); v != "" { t.Errorf("got %q", v) }
	if v := lastNonEmptyLine([]string{}); v != "" { t.Errorf("got %q", v) }
}

func TestIsPythonException(t *testing.T) {
	if !isPythonException("ValueError") { t.Error("expected true") }
	if !isPythonException("TypeError") { t.Error("expected true") }
	if !isPythonException("KeyError") { t.Error("expected true") }
	if isPythonException("RandomText") { t.Error("expected false") }
}

func TestIsLikelyFilePath(t *testing.T) {
	if !isLikelyFilePath("src/app.ts") { t.Error("expected true for src/app.ts") }
	if !isLikelyFilePath("/abs/path/file.js") { t.Error("expected true for /abs/path/file.js") }
	if !isLikelyFilePath("./relative/file.tsx") { t.Error("expected true for ./relative/file.tsx") }
	if isLikelyFilePath("-flag") { t.Error("expected false for -flag") }
	if isLikelyFilePath("") { t.Error("expected false for empty") }
}
