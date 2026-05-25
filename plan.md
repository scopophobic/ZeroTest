plan.md: Implementing Zero-Wrap Local MCP Server (Go)

This plan outlines the sequential phases to build **Zero-Wrap**—a universal, machine-optimized diagnostic middleware wrapper that converts messy, human-centric compiler errors into deterministic JSON contracts for AI agents using the official Model Context Protocol (MCP) Go SDK.

---
:
## 📋 Architectural Overview
1. **Host Environment:** Local machine only, communicating via standard I/O (`stdio`) pipes.
2. **Core Dependencies:** `github.com/modelcontextprotocol/go-sdk/mcp`
3. **Execution Loop:** Agent calls `run_diagnostic_check` via MCP -> Go server invokes the target language runner -> Intercepts `stderr` -> Matches against dynamic regex/AST recipes -> Returns zero-fluff JSON payload to the Agent.

---

## 🗂 State Tracking File Initialization
Before executing any development tasks, create an `.agent` file in the root workspace directory to track progress, state mutations, and error taxonomy rules.

### Task 0: Create `.agent` File
Initialize `.agent` with the following baseline JSON:
```json
{
  "project": "Zero-Wrap",
  "status": "bootstrapping",
  "version": "0.1.0",
  "architecture": {
    "runtime": "Go 1.22+",
    "transport": "stdio",
    "shims_loaded": []
  },
  "todo_backlog": [
    "Phase 1: Project Setup & SDK Initialization",
    "Phase 2: Universal Subprocess Execution Engine",
    "Phase 3: Recipe Configuration Pipeline (YAML/JSON)",
    "Phase 4: Python Runtime Exception Shim Implementation",
    "Phase 5: Local Integration & Registration Validation"
  ]
}
🚀 Execution Phases
Phase 1: Project Setup & SDK Initialization
[ ] Run go mod init zero-wrap.
[ ] Fetch the official MCP Go SDK: go get github.com/modelcontextprotocol/go-sdk.
[ ] Implement a baseline main.go file initializing an MCP stdio server wrapper using the generic mcp.NewServer() factory.
[ ] Expose a single schema-validated tool definition called run_diagnostic_check.
Input parameters required from agent: command []string (e.g., ["python3", "app.py"]), workspace_path string.
Expected output format: Structured stringified JSON containing the diagnostic payload.
Phase 2: Universal Subprocess Execution Engine
[ ] Build an execution service using Go's native os/exec package.
[ ] Implement context timeouts (context.WithTimeout) to ensure a rogue process hung up in the target compiler does not hang the agent loop (default threshold: 5000ms).
[ ] Create a safe data capture mechanism for stdout and stderr buffers. Even if the process returns a non-zero exit status code, capture the raw terminal trace.
Phase 3: Recipe Configuration Pipeline
[ ] Create a structured configuration file format (/recipes/python.json or /recipes/typescript.json).
[ ] Define a Go model for the mapping rules matching this schema:
Go
type ErrorRule struct {
    ID             string `json:"id"`
    Pattern        string `json:"pattern"` // Regex containing named capture groups (?P<file>, ?P<line>, ?P<msg>)
    Message        string `json:"message"`
    RepairStrategy string `json:"repair_strategy"`
}
[ ] Implement a directory scanning module inside Go that dynamically compiles these regex patterns at boot time.
Phase 4: Python Runtime Exception Shim Implementation
[ ] Write a baseline test file test_crash.py that intentionally triggers a KeyError or an IndexError.
[ ] Populate /recipes/python.json with the matching regex rule extracting error information.
[ ] Write the engine mapping routine that matches the collected stderr against loaded regular expressions, parsing named capture groups into file names, absolute paths, line offsets, and payload attributes.
Phase 5: Local Integration & Validation
[ ] Package the server runtime inside your main handler loop to format the response into the finalized schema contract.
[ ] Print structural JSON out via the standard mcp.CallToolResult parameters.
[ ] Update the .agent status block to completed and document the system capabilities payload.
🔍 Target JSON Payload Contract Verification
Ensure your tool implementation translates all intercepted data into this structural blueprint:
JSON
{
  "ok": false,
  "runtime_metadata": {
    "language": "python",
    "toolchain": "runtime",
    "execution_time_ms": 12
  },
  "diagnostics": [
    {
      "code": "ZW_PY_INDEX_001",
      "severity": "error",
      "message": "list index out of range",
      "location": {
        "file": "/absolute/path/to/test_crash.py",
        "line": 4,
        "column": null
      },
      "repair_hint": {
        "strategy": "bounds_check_insertion",
        "payload": {}
      }
    }
  ]
}
🛠 Direct Instructions for AI Assistant Execution
Create the .agent tracking template right now.
Draft the initialization code for main.go using github.com/modelcontextprotocol/go-sdk.
Provide step-by-step terminal installation snippets for local testing.
***

### Why this plan file is ready to drop right in:
* **Strict boundaries:** It uses the exact official repo path (`github.com/modelcontextprotocol/go-sdk`) so your copilot won't accidentally invent package structures from third-party wrappers or old implementations.
* **Separated Phases:** It explicitly instructs the model to complete the scaffolding and structural engine code before writing any complex regex matching rules.
