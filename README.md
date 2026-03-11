# opengrok-mcp-server

An MCP server that lets MCP-compatible clients such as Claude Code or Copilot
in VS Code search source code indexed by
[OpenGrok](https://github.com/oracle/opengrok). Instead of writing Lucene
queries by hand, you ask natural language questions and the MCP client can
search OpenGrok on your behalf.

## Why this is useful

If you only need to search one local repository on your own machine, your IDE's
built-in search, symbol navigation, or `rg` may be simpler.

This project becomes useful when you want:

- AI tools to search an existing OpenGrok index through MCP instead of opening every repo locally
- a shared search service for large or multiple repositories
- better code-search primitives for agents, such as "find definitions", "find references", and "read this file"
- access to centrally indexed codebases that are too large or inconvenient to clone into every editor workspace
- a thin bridge between OpenGrok and MCP clients, so the same indexed source can be used from different tools

In short: local IDE search is great for local development, while this server is
useful when OpenGrok is already the source of truth for searchable code and you
want AI clients to use it directly.

## How it works

```
You ask your MCP client a question
  → the client picks the right search tool
    → MCP server queries OpenGrok
      → OpenGrok searches its Lucene index
        → Results come back to the client
          → the client reads the code and answers
```

## Tools exposed to MCP clients

| Tool | What it searches |
|---|---|
| `search_code` | Full-text — any occurrence of the query inside file contents |
| `search_definition` | Only where a symbol (function, class, etc.) is *defined* |
| `search_references` | Only where a symbol is *called or used* |
| `search_file` | File path/name patterns |
| `get_file_content` | Fetches the complete source of a specific file |
| `list_projects` | Lists all projects indexed by OpenGrok |

---

## Setup

### 1. Create your source directory

OpenGrok treats each subdirectory of its source folder as a separate **project**.
Clone any repos you want indexed into it:

```bash
mkdir -p ~/opengrok/src
cd ~/opengrok/src

# Each repo becomes one searchable project.
git clone <your-repo-url>
```

**Example — indexing the Jenkins source code:**

```bash
mkdir -p ~/opengrok/src
cd ~/opengrok/src
git clone https://github.com/jenkinsci/jenkins
```

This gives OpenGrok a single project called `jenkins`. Add as many repos as you
like — each subdirectory becomes a project.

---

### 2. Start everything with Docker Compose

```bash
git clone https://github.com/8ball030/opengrok-mcp-server
cd opengrok-mcp-server
docker compose up -d
```

This starts two containers on a shared Docker network (`opengrok-net`):

| Container | Role |
|---|---|
| `opengrok` | OpenGrok web app + indexer. UI on [http://localhost:8080](http://localhost:8080) |
| `opengrok-mcp` | MCP server binary, kept alive for an MCP client to exec into |

The MCP server calls OpenGrok at `http://opengrok:8080` — the service name
resolves automatically within the Docker network, no localhost forwarding needed.

**Wait for indexing to finish (~2–3 min for Jenkins), then verify:**

```bash
curl http://localhost:8080/api/v1/projects
# Should return: ["jenkins"]
```

**Re-index after pulling new commits:**

```bash
docker compose restart opengrok
```

**Stop everything:**

```bash
docker compose down
```

---

### 3. Register with an MCP client

Example with Claude Code:

```bash
claude mcp add opengrok -s user -- docker exec -i opengrok-mcp /opengrok-mcp-server
```

The `-s user` flag makes it available in all your projects, not just the current one.
`docker exec -i opengrok-mcp` runs the binary inside the already-running container
using stdin/stdout — no port forwarding or extra config needed.

Verify it registered:

```bash
claude mcp list
```

---

## Usage

Once registered, ask your MCP client questions in plain English:

```
"What projects are indexed?"
"Where is the HTTP timeout configured in Jenkins?"
"Find the definition of the Executor class"
"What calls the Queue.maintain() method?"
"Show me all files named *Plugin.java"
"Read the full source of src/main/java/jenkins/model/Jenkins.java in the jenkins project"
```

The client can decide which tool to call, run the search, and answer based on
the actual source code — no manual query writing needed.

---

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `OPENGROK_URL` | Yes | URL of OpenGrok (set automatically in Docker Compose) |
| `OPENGROK_TOKEN` | No | Bearer token if your OpenGrok instance requires auth |

---

## Project structure

```
opengrok-mcp-server/
├── cmd/server/main.go          # MCP server entry point and tool definitions
├── internal/opengrok/client.go # OpenGrok REST API client
├── config/
│   └── opengrok-readonly.xml   # Read-only config merged into OpenGrok at startup
├── Dockerfile                  # Multi-stage Go build → alpine runtime image
├── docker-compose.yml          # OpenGrok + MCP server on shared network
├── go.mod
└── README.md
```
