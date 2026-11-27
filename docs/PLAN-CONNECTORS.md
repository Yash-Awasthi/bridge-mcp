# plan.md — connectors: any local MCP server, optional

**Status:** implemented (`pkg/connectors`, `pkg/api/connectors.go`, `cmd/open-board/connectors.go`).
The board stays exactly what it is today. A *connector* is just another MCP server running on
this machine that the board can call. Rubrik is one preset, not a dependency. Nothing here is
Rubrik-specific code.

- No connectors configured → no child processes, no new network calls, no changed behaviour.
- One connector configured → one extra set of tools for every agent, in every editor.
- Works offline, stays local, one binary. The child process is optional and user-supplied.

---

## 1. Config (`.board/connectors.json`, optional)

```json
{
  "connectors": {
    "rubrik": {
      "command": "uvx",
      "args": ["rubrik-mcp"],
      "deny": ["rsc_take_on_demand_snapshot", "rsc_assign_sla", "rsc_onboard_host",
               "rsc_save_workflow", "rsc_delete_workflow"]
    },
    "github": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"] }
  }
}
```

- `command` + `args` + optional `env` (name → value **reference**, never a secret in the file),
  optional `deny` (tool names refused) and optional `idle_seconds` (default 300).
- Credentials come from the parent environment (`RSC_SERVICE_ACCOUNT_FILE`, `GITHUB_TOKEN`, …).
  The board passes the environment through and never reads, logs or stores a secret.
- Adding a connector is one command:

```bash
vb connector add github npx -y @modelcontextprotocol/server-github
vb connector add rubrik --preset rubrik     # fills command + deny list
```

Presets are a small table in code (`rubrik`, and room for more). A preset only supplies a
default command and a deny list; you can always edit the JSON by hand instead.

## 2. Runtime

| Piece | Size | What it does |
|---|---|---|
| `pkg/connectors/stdio.go` | ~200 lines | start a child, `initialize`, `tools/list`, `tools/call`, timeout, kill on close |
| `pkg/connectors/connectors.go` | ~200 lines | load `connectors.json`, lazy start, tool cache, deny list, idle stop |
| `pkg/connectors/presets.go` | ~40 lines | `rubrik` preset (command + deny list) only |
| `vb connector` CLI | ~120 lines | `add`, `remove`, `list`, `tools`, `call` |
| API + MCP tools | ~120 lines | `GET /api/connectors`, `POST /api/connectors/call`, `board_connectors`, `board_connector_call` |
| tests | ~200 lines | stub stdio server: handshake, list, call, deny, error, timeout, idle stop |

No new Go dependencies. About 700–800 lines including tests.

Behaviour:

- **Lazy:** a child starts on the first call for that connector and is stopped after
  `idle_seconds` with no calls. `vb serve` never spawns anything at startup.
- **Allowed = what the server reports.** The tool list comes from the child's `tools/list`;
  a name it does not expose is refused. `deny` is applied on top, so a new write tool in a
  future server release stays unreachable if you listed it.
- **One note, no data hoarding.** A call may post one line to a thread
  (`connector: rubrik/rsc_get_clusters ok in 1.9s`). Results go back to the caller and are not
  written to SQLite unless the caller chooses to post them as a normal message.
- **Timeouts:** 10 s handshake, 60 s per call (configurable), child killed on broker shutdown.
- **Clear failure:** missing command → `connector "rubrik" is not installed: run "uvx rubrik-mcp"
  or edit .board/connectors.json`; no panic, no retry loop.
- **Universally available:** `board_connectors` and `board_connector_call` are ordinary
  `board_*` tools, so every client `vb install` supports gets them with no new config. Any app
  outside the registry still works with the `vb install --dry-run` snippet.

## 3. Steps

1. **stdio client.** `pkg/connectors/stdio.go` + tests against a helper process.
   *Done when:* handshake, `tools/list`, `tools/call` and timeout pass in `go test`.
2. **Config + lifecycle.** Load/validate `connectors.json`, lazy start, idle stop, deny list.
   *Done when:* a bad file gives one readable error; an absent file changes nothing.
3. **CLI.** `vb connector add|remove|list|tools|call`.
   *Done when:* `vb connector add rubrik --preset rubrik && vb connector tools rubrik` prints
   live tool names, and `vb connector call rubrik rsc_get_clusters` prints JSON.
4. **Broker + MCP tools.** Routes and the two `board_*` tools, with opts off by default.
   *Done when:* Claude Desktop / Cursor / any installed client can call a connector, and an
   unknown or denied tool is refused with the allowed list in the error.
5. **Docs.** A short "Connectors" section in `docs/INSTALL.md` and three lines in `README.md`.
   *Done when:* `bash scripts/check-docs.sh` passes and Rubrik appears only as an example.

## 4. Try it

```bash
./vb serve
./vb connector add rubrik --preset rubrik
export RSC_SERVICE_ACCOUNT_FILE=/path/to/service_account.json
./vb connector tools rubrik
./vb connector call rubrik rsc_get_clusters
```

In an editor that already has the board: *"List my board connectors, then call
`rsc_get_workloads` on rubrik with compliance_status OUT_OF_COMPLIANCE."*

## 5. Checks

- `go build ./...`, `go test ./...`, `go vet ./...`, `bash scripts/check-docs.sh`.
- With no `connectors.json`: no process starts, no route answers anything but an empty list,
  and the existing suite is untouched.
- With a bad command or no credentials: one clear error from the child, recorded in
  `daemon.log`, never a panic or a hang.

## 6. Deliberately out of scope

Accounts, RBAC, approvals, Postgres, containers, audit chains, telemetry. Those turn a local
bridge into a platform, and they are not needed for a person and their agents on one machine.
