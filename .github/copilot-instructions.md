# OpenGrok MCP Server — Copilot Instructions

## Project Overview

This is an MCP (Model Context Protocol) server that exposes OpenGrok — a source code search engine with Lucene indexing — as a tool interface for Claude. Users ask questions in natural language; Claude picks the appropriate search tool and queries OpenGrok on their behalf.

**Core flow:** User question → Claude selects tool → MCP server calls OpenGrok REST API → Results returned to Claude

## Architecture

### Key Components

- **[cmd/server/main.go](cmd/server/main.go)** — MCP server bootstrap. Defines 6 tools (`search_code`, `search_definition`, `search_references`, `search_file`, `get_file_content`, `list_projects`). Each tool maps Claude's request to OpenGrok API parameters.

- **[internal/opengrok/client.go](internal/opengrok/client.go)** — HTTP client for OpenGrok's REST API (`/api/v1/search`, `/api/v1/file/content`, `/api/v1/projects`). Handles authentication (optional Bearer token), type conversions (e.g., `lineNumber` is returned as JSON string), and error handling.

- **[Dockerfile](Dockerfile)** — Multi-stage Go build. Creates a statically-linked binary, runs in alpine. Binary stays alive via `sleep infinity` in compose; Claude execs into container to invoke.

- **[docker-compose.yml](docker-compose.yml)** — Two services on shared `opengrok-net`:
  - `opengrok`: OpenGrok 1.7.32 (indexes source, serves web UI on 8080)
  - `mcp-server`: This server (no exposed ports; Claude execs into it)

## Development Workflows

### Local Testing

```bash
# Start both containers (OpenGrok + MCP server)
docker compose up -d

# Wait for OpenGrok to index (2–3 min for larger repos), verify:
curl http://localhost:8080/api/v1/projects

# Test the binary directly (outside Docker):
OPENGROK_URL=http://localhost:8080 go run ./cmd/server
```

### Adding/Modifying Tools

1. Define tool shape in [cmd/server/main.go](cmd/server/main.go) using `mcp.NewTool()` with parameter definitions.
2. Implement handler closure that extracts params via `req.GetString()`, `req.GetInt()`, etc.
3. Call `og.Search()` or `og.GetFileContent()` from the client; use `runSearch()` helper for formatting.
4. Return results via `mcp.NewToolResultText()` or `mcp.NewToolResultError()`.

### Build & Deployment

```bash
# Build the Go binary
go build -o opengrok-mcp-server ./cmd/server/

# Docker builds automatically via docker compose up -d
# To rebuild after code changes:
docker compose down && docker compose up -d --build
```

## Project-Specific Patterns

### Search Tools vs. File Fetching

- **Search tools** (`search_code`, `search_definition`, `search_references`, `search_file`) are coarse filters. They return partial hits (matching lines). They accept `max_results` to limit volume.
- **`get_file_content`** is the fine-grained reader. Claude calls it after identifying a file via search, to read the full implementation.

### SearchParams & Query Building

The `SearchParams` struct in [client.go](internal/opengrok/client.go) maps cleanly to OpenGrok's URL query params:
- `Query` → `?q=`
- `Defs` → `?defs=`
- `Refs` → `?refs=`
- `Path` → `?path=`
- `Projects` → `?projects=` (multi-value)

Tools in [main.go](cmd/server/main.go) construct these structs from Claude's tool calls and pass them to `og.Search()`.

### Line Number Handling

OpenGrok returns `lineNumber` as a JSON string, not an int. The client manually parses it (`strconv.Atoi`) to avoid marshalling errors. See `hitJSON` vs. `Hit` in [client.go](internal/opengrok/client.go).

### Error Handling

All tool handlers wrap OpenGrok errors as tool result errors (`mcp.NewToolResultError()`). Empty results return `"No results found."` rather than erroring.

## Configuration & External Dependencies

- **Environment variables:**
  - `OPENGROK_URL` (required) — e.g., `http://opengrok:8080` (within compose) or `http://localhost:8080` (local testing)
  - `OPENGROK_TOKEN` (optional) — Bearer token for authenticated OpenGrok instances

- **Go dependencies:**
  - `github.com/mark3labs/mcp-go/mcp` — MCP protocol types and server
  - `github.com/mark3labs/mcp-go/server` — MCP server framework
  - Standard library: `net/http`, `encoding/json`, `url`, etc.

- **Docker:**
  - OpenGrok container auto-reindexes every 60 min (configurable via `SYNC_PERIOD_MINUTES`)
  - Source mounted at `${OPENGROK_SRC:-~/opengrok/src}`

## Testing & Debugging

- **Unit testing:** No formal test suite yet; [test_grok.go](test_grok.go) is a manual HTTP client example. Consider adding integration tests that start a local OpenGrok instance.
- **Debugging MCP:** Enable logging in `main()` with `log.Println()` calls. Logs appear in compose logs: `docker compose logs mcp-server`.
- **OpenGrok web UI:** Verify indexing and run manual searches at `http://localhost:8080/source` to compare against tool output.
