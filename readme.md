# Diagnostic Translator (MCP)

A Go-based MCP server that runs a command and translates terminal output into structured JSON diagnostics.

## Run

```bash
go run .
```

## Build

```bash
go build -o diagnostic-translator .
```

## MCP client setup (stdio)

Configure your MCP client to launch the server over stdio:

```json
{
  "command": "go",
  "args": ["run", "."]
}
```

## Tool: `diagnose_command`

**Arguments**
- `command` (string, required): Executable to run (for example `python`)
- `args` (string[], optional): Arguments to pass to the command
- `cwd` (string, optional): Working directory
- `timeout_ms` (integer, optional): Timeout in milliseconds (default 120000)
- `language_hint` (string, optional): Override detected language
- `toolchain_hint` (string, optional): Override detected toolchain

Arguments are passed without shell evaluation. Example:

```json
{
  "command": "python",
  "args": ["manage.py", "runserver", "-json"]
}
```

**Example request**
```json
{
  "command": "python",
  "args": ["-m", "mypy", "src/app.py"],
  "timeout_ms": 60000
}
```

**Example response**
```json
{
  "ok": false,
  "runtime_metadata": {
    "language": "python",
    "toolchain": "mypy",
    "execution_time_ms": 14,
    "exit_code": 1,
    "command": "python",
    "args": ["-m", "mypy", "src/app.py"]
  },
  "diagnostics": [
    {
      "code": "DT_PY_TYPE",
      "severity": "error",
      "message": "Unsupported operand types for +: \"int\" and \"str\"",
      "location": {
        "file": "src/app.py",
        "line": 42,
        "column": 18
      },
      "context": {
        "source_line": "total_count = items_count + baseline_string"
      }
    }
  ]
}
```

**Response fields**
- `ok`: whether the command succeeded
- `runtime_metadata`: execution details (language, toolchain, timing, exit code, command)
- `diagnostics`: structured issues derived from stderr/stdout
- `raw_output`, `stdout`, `stderr`: captured output (truncated if large)
