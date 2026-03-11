# opengrok-mcp-server

An MCP server that lets MCP-compatible clients such as Cursor, VS Code, Claude
Code, or Copilot query source code indexed by
[OpenGrok](https://github.com/oracle/opengrok).

Instead of making an agent scan the filesystem on every request, this server
uses OpenGrok's index for fast code search, definition lookup, reference lookup,
file lookup, and file reads.

## When to use it

If you only need to search one local repository, your IDE search, symbol
navigation, or `rg` may be simpler.

This server is useful when:

- OpenGrok is already your source of truth for searchable code
- you want AI clients to search large indexed repositories quickly
- you want structured tools like definition search, reference search, and file reads
- you want multiple clients to use the same indexed codebase

## Tools

| Tool | Purpose |
|---|---|
| `search_code` | Full-text search inside file contents |
| `search_definition` | Find where a symbol is defined |
| `search_references` | Find where a symbol is used |
| `search_file` | Search by file path or name |
| `get_file_content` | Read a file from the indexed source tree |
| `list_projects` | List indexed OpenGrok projects |

## Quick start

### 1. Put repositories under the OpenGrok source root

Each subdirectory becomes one OpenGrok project.

```bash
mkdir -p ~/opengrok/src
cd ~/opengrok/src
git clone https://github.com/jenkinsci/jenkins
```

That creates an indexed project named `jenkins`.

### 2. Start OpenGrok and the MCP server

```bash
git clone https://github.com/8ball030/opengrok-mcp-server
cd opengrok-mcp-server
docker compose up -d
```

This starts:

- `opengrok` on `http://localhost:8080`
- `opengrok-mcp`, a container kept alive so your MCP client can `docker exec` into it

### 3. Wait for indexing, then verify

The local demo OpenGrok in this repo is token-protected. Use the bundled demo
token when checking it from the host:

```bash
curl -H "Authorization: Bearer mcp-secret-code-123" \
  http://localhost:8080/api/v1/projects
```

Expected response:

```json
["jenkins"]
```

### 4. Connect an MCP client

Example with Claude Code:

```bash
claude mcp add opengrok -s user -- docker exec -i opengrok-mcp /opengrok-mcp-server
```

Example project config for Cursor in `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "opengrok": {
      "command": "docker",
      "args": ["exec", "-i", "opengrok-mcp", "/opengrok-mcp-server"]
    }
  }
}
```

Example project config for VS Code in `.vscode/mcp.json`:

```json
{
  "servers": {
    "opengrok": {
      "type": "stdio",
      "command": "docker",
      "args": ["exec", "-i", "opengrok-mcp", "/opengrok-mcp-server"]
    }
  }
}
```

Committing these config files is optional. They are safe to share when they do
not contain secrets or machine-specific paths.

## Example questions

Once connected, ask your MCP client things like:

```text
What projects are indexed?
Find the definition of the Executor class in Jenkins.
What calls Queue.maintain()?
Show me all files named *Plugin.java.
Read the full source of core/src/main/java/hudson/model/Executor.java in the jenkins project.
```

## Using a remote OpenGrok server

You do not need the bundled local OpenGrok container if your code is already
indexed elsewhere.

Set:

- `OPENGROK_URL` to the remote OpenGrok base URL
- `OPENGROK_TOKEN` to a Bearer token accepted by that server

Example:

```bash
OPENGROK_URL="https://opengrok.example.com" \
OPENGROK_TOKEN="your-real-token" \
go run ./cmd/server
```

Or with Docker:

```bash
docker run --rm -i \
  -e OPENGROK_URL="https://opengrok.example.com" \
  -e OPENGROK_TOKEN="your-real-token" \
  opengrok-mcp-server
```

Then point your MCP client at that command instead of the local
`docker exec -i opengrok-mcp /opengrok-mcp-server` example.

Notes:

- `config/opengrok-readonly.xml` is only used by the local demo OpenGrok container
- this server currently supports Bearer-token auth to OpenGrok
- the machine running this MCP server must be able to reach and trust the remote OpenGrok server

## Useful commands

Re-index after pulling source changes:

```bash
docker compose restart opengrok
```

Stop everything:

```bash
docker compose down
```

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `OPENGROK_URL` | Yes | Base URL of the OpenGrok instance |
| `OPENGROK_TOKEN` | No | Bearer token for authenticated OpenGrok instances |

## Project structure

```text
opengrok-mcp-server/
├── cmd/server/main.go
├── internal/opengrok/client.go
├── config/opengrok-readonly.xml
├── Dockerfile
├── docker-compose.yml
└── README.md
```
