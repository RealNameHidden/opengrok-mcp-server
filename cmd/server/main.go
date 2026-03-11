package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/8ball030/opengrok-mcp-server/internal/opengrok"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// --- Config from environment variables -----------------------------------
	//
	// OPENGROK_URL   (required) – root URL of your OpenGrok instance,
	//                e.g. "http://localhost:8080/source"
	// OPENGROK_TOKEN (optional) – Bearer token for authenticated instances
	//
	opengrokURL := os.Getenv("OPENGROK_URL")
	if opengrokURL == "" {
		log.Fatal("OPENGROK_URL environment variable is required")
	}
	opengrokToken := os.Getenv("OPENGROK_TOKEN") // may be empty

	// --- OpenGrok client -----------------------------------------------------
	og := opengrok.NewClient(opengrokURL, opengrokToken)

	// Fetch project names at startup so we can expose them as enum values,
	// which causes MCP clients to render a dropdown for the project parameter.
	var projectNames []string
	if projects, err := og.ListProjects(); err == nil {
		for _, p := range projects {
			projectNames = append(projectNames, p.Name)
		}
	} else {
		log.Printf("warning: could not fetch projects for enum: %v", err)
	}

	projectOption := func() mcp.PropertyOption {
		if len(projectNames) > 0 {
			return mcp.Enum(projectNames...)
		}
		return mcp.Description("Limit to a specific OpenGrok project name. Omit to search all projects.")
	}

	// --- MCP server ----------------------------------------------------------
	s := server.NewMCPServer(
		"opengrok-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	// =========================================================================
	// Tool: search_code
	//
	// Full-text search — looks inside file contents for any occurrence of
	// the query string. The broadest of the four search tools.
	// =========================================================================
	s.AddTool(
		mcp.NewTool("search_code",
			mcp.WithDescription(
				"Search source code using full-text search across indexed projects in OpenGrok. "+
					"Returns matching file paths and the specific lines that matched. "+
					"Use this when you want to find where a string, function call, or pattern appears in code.",
			),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Full-text search query. Supports Lucene syntax, e.g. \"http.Get\"."),
			),
			mcp.WithString("project",
				mcp.Description("Limit to a specific OpenGrok project name. Omit to search all projects."),
				projectOption(),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum results to return. Defaults to 25."),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query := req.GetString("query", "")
			if query == "" {
				return mcp.NewToolResultError("query parameter is required"), nil
			}
			params := opengrok.SearchParams{
				Query:      query,
				MaxResults: req.GetInt("max_results", 25),
			}
			if p := req.GetString("project", ""); p != "" {
				params.Projects = []string{p}
			}
			return runSearch(og, params)
		},
	)

	// =========================================================================
	// Tool: search_definition
	//
	// Searches the definitions index — only matches places where a symbol
	// (function, class, constant, etc.) is *defined*, not where it is used.
	// Much more precise than full-text when you want "show me the implementation
	// of X" rather than "show me everywhere X is mentioned".
	// =========================================================================
	s.AddTool(
		mcp.NewTool("search_definition",
			mcp.WithDescription(
				"Search for where a symbol is *defined* in the indexed source code. "+
					"Use this to find the implementation of a function, class, or constant. "+
					"More precise than search_code because it only matches definition sites.",
			),
			mcp.WithString("symbol",
				mcp.Required(),
				mcp.Description("The symbol name to look up, e.g. \"NewClient\" or \"HttpHandler\"."),
			),
			mcp.WithString("project",
				mcp.Description("Limit to a specific OpenGrok project name. Omit to search all projects."),
				projectOption(),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum results to return. Defaults to 25."),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			symbol := req.GetString("symbol", "")
			if symbol == "" {
				return mcp.NewToolResultError("symbol parameter is required"), nil
			}
			params := opengrok.SearchParams{
				Defs:       symbol,
				MaxResults: req.GetInt("max_results", 25),
			}
			if p := req.GetString("project", ""); p != "" {
				params.Projects = []string{p}
			}
			return runSearch(og, params)
		},
	)

	// =========================================================================
	// Tool: search_references
	//
	// Searches the references index — only matches places where a symbol is
	// *used or called*, not where it is defined.
	// Great for "show me everything that calls validateUser" or impact analysis
	// before refactoring a function.
	// =========================================================================
	s.AddTool(
		mcp.NewTool("search_references",
			mcp.WithDescription(
				"Search for where a symbol is *used or referenced* in the indexed source code. "+
					"Use this to find all callers of a function, or all usages of a variable. "+
					"Complements search_definition: one finds the impl, this finds the call sites.",
			),
			mcp.WithString("symbol",
				mcp.Required(),
				mcp.Description("The symbol name to search references for, e.g. \"validateUser\"."),
			),
			mcp.WithString("project",
				mcp.Description("Limit to a specific OpenGrok project name. Omit to search all projects."),
				projectOption(),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum results to return. Defaults to 25."),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			symbol := req.GetString("symbol", "")
			if symbol == "" {
				return mcp.NewToolResultError("symbol parameter is required"), nil
			}
			params := opengrok.SearchParams{
				Refs:       symbol,
				MaxResults: req.GetInt("max_results", 25),
			}
			if p := req.GetString("project", ""); p != "" {
				params.Projects = []string{p}
			}
			return runSearch(og, params)
		},
	)

	// =========================================================================
	// Tool: search_file
	//
	// Searches by file path pattern rather than file contents.
	// Useful when you know roughly what the file is called but not where it lives,
	// e.g. "find all files named *_test.go" or "find config.yaml".
	// =========================================================================
	s.AddTool(
		mcp.NewTool("search_file",
			mcp.WithDescription(
				"Search for files by name or path pattern across indexed projects. "+
					"Use this when you know (part of) a filename but not its location, "+
					"e.g. \"config.go\" or \"*_handler.go\". Returns matching file paths.",
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("File path pattern to search for, e.g. \"config.go\" or \"*/handler*\"."),
			),
			mcp.WithString("project",
				mcp.Description("Limit to a specific OpenGrok project name. Omit to search all projects."),
				projectOption(),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum results to return. Defaults to 25."),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			path := req.GetString("path", "")
			if path == "" {
				return mcp.NewToolResultError("path parameter is required"), nil
			}
			params := opengrok.SearchParams{
				Path:       path,
				MaxResults: req.GetInt("max_results", 25),
			}
			if p := req.GetString("project", ""); p != "" {
				params.Projects = []string{p}
			}
			return runSearch(og, params)
		},
	)

	// =========================================================================
	// Tool: get_file_content
	//
	// Fetches the complete raw source of a single file.
	// Claude typically calls this after a search tool identifies an interesting
	// file — search narrows it down, this reads the full thing.
	// =========================================================================
	s.AddTool(
		mcp.NewTool("get_file_content",
			mcp.WithDescription(
				"Fetch the full source code of a specific file from an OpenGrok project. "+
					"Use this after a search identifies a relevant file and you need to read "+
					"the complete implementation rather than just the matching lines.",
			),
			mcp.WithString("project",
				mcp.Required(),
				mcp.Description("The OpenGrok project name that contains the file."),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("The file path within the project, e.g. \"/src/client.go\"."),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			project := req.GetString("project", "")
			path := req.GetString("path", "")
			if project == "" || path == "" {
				return mcp.NewToolResultError("both project and path parameters are required"), nil
			}

			content, err := og.GetFileContent(project, path)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to fetch file: %v", err)), nil
			}
			return mcp.NewToolResultText(content), nil
		},
	)

	// =========================================================================
	// Tool: list_projects
	//
	// Returns every project indexed by this OpenGrok instance.
	// Claude should call this first when the user hasn't specified a project,
	// so it knows what's available before searching.
	// =========================================================================
	s.AddTool(
		mcp.NewTool("list_projects",
			mcp.WithDescription(
				"List all source code projects indexed by this OpenGrok instance. "+
					"Call this first to discover available project names before searching, "+
					"or when the user asks what codebases are available.",
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			projects, err := og.ListProjects()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list projects: %v", err)), nil
			}
			if len(projects) == 0 {
				return mcp.NewToolResultText("No projects are currently indexed."), nil
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "%d project(s) indexed:\n\n", len(projects))
			for _, p := range projects {
				fmt.Fprintf(&sb, "- %s", p.Name)
				if p.Description != "" {
					fmt.Fprintf(&sb, ": %s", p.Description)
				}
				sb.WriteString("\n")
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// --- Start the server on stdio -------------------------------------------
	//
	// stdio is the standard transport for Claude Desktop / Claude Code.
	// The MCP client (Claude) spawns this binary as a subprocess and
	// communicates over stdin/stdout using JSON-RPC.
	//
	log.Println("opengrok-mcp server starting on stdio")
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// runSearch is a shared helper used by all four search tools.
// It calls OpenGrok, handles the empty-result case, and formats
// the hits into readable text for Claude.
func runSearch(og *opengrok.Client, params opengrok.SearchParams) (*mcp.CallToolResult, error) {
	results, err := og.Search(params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("OpenGrok search failed: %v", err)), nil
	}
	if results.ResultCount == 0 {
		return mcp.NewToolResultText("No results found."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d result(s) across %d file(s):\n\n",
		results.ResultCount, len(results.Results))

	for filePath, hits := range results.Results {
		fmt.Fprintf(&sb, "### %s\n", filePath)
		for _, hit := range hits {
			fmt.Fprintf(&sb, "  Line %d: %s\n", hit.LineNumber, hit.Line)
		}
		sb.WriteString("\n")
	}
	return mcp.NewToolResultText(sb.String()), nil
}
