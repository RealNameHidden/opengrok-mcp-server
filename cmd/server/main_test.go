package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/8ball030/opengrok-mcp-server/internal/opengrok"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestRunSearchFormatsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"resultCount": 2,
			"startDocument": 0,
			"endDocument": 2,
			"time": 9,
			"results": {
				"/src/main.go": [
					{"line":"func <b>main</b>() {}", "lineNumber":"10", "tag":"method"}
				],
				"/src/lib.go": [
					{"line":"call <b>main</b>()", "lineNumber":"27", "tag":"reference"}
				]
			}
		}`))
	}))
	defer server.Close()

	result, err := runSearch(opengrok.NewClient(server.URL, ""), opengrok.SearchParams{Query: "main"})
	if err != nil {
		t.Fatalf("runSearch returned error: %v", err)
	}
	if result.IsError {
		t.Fatal("runSearch returned error result, want success")
	}

	text := toolResultText(t, result)
	for _, want := range []string{
		"Found 2 result(s) across 2 file(s):",
		"### /src/main.go",
		"Line 10: func <b>main</b>() {}",
		"### /src/lib.go",
		"Line 27: call <b>main</b>()",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatted text missing %q:\n%s", want, text)
		}
	}
}

func TestRunSearchNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"resultCount": 0,
			"startDocument": 0,
			"endDocument": 0,
			"time": 1,
			"results": {}
		}`))
	}))
	defer server.Close()

	result, err := runSearch(opengrok.NewClient(server.URL, ""), opengrok.SearchParams{Query: "missing"})
	if err != nil {
		t.Fatalf("runSearch returned error: %v", err)
	}
	if got := toolResultText(t, result); got != "No results found." {
		t.Fatalf("text = %q, want %q", got, "No results found.")
	}
}

func TestRunSearchSurfacesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusBadGateway)
	}))
	defer server.Close()

	result, err := runSearch(opengrok.NewClient(server.URL, ""), opengrok.SearchParams{Query: "main"})
	if err != nil {
		t.Fatalf("runSearch returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("runSearch result IsError = false, want true")
	}
	if got := toolResultText(t, result); !strings.Contains(got, "OpenGrok search failed") {
		t.Fatalf("error text = %q", got)
	}
}

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if len(result.Content) != 1 {
		t.Fatalf("len(result.Content) = %d, want 1", len(result.Content))
	}

	textContent, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatalf("result content is %T, want text", result.Content[0])
	}

	return textContent.Text
}
