# Contributing

Thanks for considering contributing to Diagnostic Translator.

## Project Structure

```
main.go            — MCP server setup, tool handler, command runner
types.go           — DiagnosticReport, Diagnostic, Location, etc.
parsers.go         — All language parsers, detection, and utilities
parsers_test.go    — Tests for every parser and utility
```

## Adding a New Language

1. **Add regex patterns** in `parsers.go` — define patterns as package-level `var` with `regexp.MustCompile`.

2. **Write a parser function** — should accept `[]string` (lines) and return `[]Diagnostic`. Follow the existing pattern:

```go
func parseFooDiagnostics(lines []string) []Diagnostic {
    diagnostics := make([]Diagnostic, 0)
    for _, line := range lines {
        line = strings.TrimSpace(line)
        // match and append Diagnostics
    }
    return diagnostics
}
```

3. **Add detection** — update `detectLanguage()` and `detectToolchain()` in `parsers.go` to recognize the language from the command name or output.

4. **Wire it in** — add your parser to `parseDiagnostics()` in both the language-specific `case` and the default fallback.

5. **Write tests** — add test cases in `parsers_test.go`:
   - Test each diagnostic format the language produces
   - Test detection by command name
   - Test that non-matching output returns no diagnostics

6. **Run tests** — `go test ./...` must pass.

### Diagnostic Code Convention

Use the `formatDiagnosticCode(prefix, code)` helper:

```go
Code: formatDiagnosticCode("DT_RS", "E0308")  // → "DT_RS_E0308"
Code: formatDiagnosticCode("DT_C", "error")    // → "DT_C_ERROR"
```

Prefixes used so far: `DT_PY`, `DT_GO`, `DT_TS`, `DT_ESLINT`, `DT_RS`, `DT_C`, `DT_JV`, `DT_RB`.

## Running Tests

```bash
go test ./... -v
```

## Pull Request Process

1. Open an issue describing what you're working on (optional but recommended).
2. Fork the repo and create a branch: `git checkout -b feature/my-feature`.
3. Make your changes and ensure all tests pass.
4. Submit a pull request with a clear description of the change.

## Reporting Issues

- Include the exact command that was run
- Include the full output (stderr/stdout)
- Include the language and toolchain version if relevant
- If possible, include a minimal reproduction

## Code Style

- Follow standard Go conventions (`gofmt`/`go vet`)
- No external dependencies beyond the MCP SDK
- Keep parser regexes readable — add comments for complex patterns if needed
- Tests use table-driven style with subtests
