# How to Use Diagnostic Translator (MCP)

## What this is
Diagnostic Translator is an MCP server that runs a command and converts its terminal output into a structured JSON diagnostics report. It is designed to make error messages easier for agents or tools to understand and fix.

## Quick start
1. Build the server:

```bash
go build -o diagnostic-translator .
```

2. Run it under an MCP client over stdio:

```json
{
  "command": "./diagnostic-translator",
  "args": []
}
```

## Using the `diagnose_command` tool
**Request shape**

```json
{
  "command": "python",
  "args": ["-m", "mypy", "src/app.py"],
  "cwd": "/workspace",
  "timeout_ms": 60000,
  "language_hint": "python",
  "toolchain_hint": "mypy"
}
```

**Example: wrapper command**

```json
{
  "command": "zerotest",
  "args": ["python", "manage.py", "runserver", "-json"]
}
```

## Response interpretation
The tool returns structured content (JSON) with these key fields:

- `ok`: `true` if the command succeeded.
- `runtime_metadata`: execution timing, exit code, and detected language/toolchain.
- `diagnostics`: structured issues parsed from stderr/stdout.
- `raw_output`, `stdout`, `stderr`: captured output, truncated if too large.

If parsing fails, a generic diagnostic (`DT_GENERIC`) is returned with the best available message.

## Production guidance
**Deployment model**
Run the server as a long-lived stdio subprocess owned by your MCP client. For production, prefer a compiled binary and a process supervisor (systemd, supervisord, or container runtime).

**Security**
This server executes arbitrary commands. In production, do all of the following:
- Run under a dedicated low-privilege user.
- Restrict allowed commands (wrap/allowlist at the MCP client or replace `command` with a fixed runner).
- Use isolated working directories and environment variables.

**Performance and limits**
- Default timeout is 120000 ms. Set `timeout_ms` explicitly for long or risky commands.
- Output is truncated to avoid oversized payloads. Treat `raw_output` as best-effort context.

**Observability**
Capture MCP logs and stderr of the server process. Store diagnostic JSON for downstream analysis.

## Workflow integration patterns
Use the tool in these common flows:
- **CI diagnostics:** run build/test commands and ingest structured errors.
- **Agent triage:** feed diagnostics directly into an agent loop for automated fixes.
- **Local dev:** run failing commands and inspect normalized diagnostics for faster debugging.
