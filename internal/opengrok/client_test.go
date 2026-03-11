package opengrok

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	client := NewClient("http://example.com/", "token")

	if client.baseURL != "http://example.com" {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, "http://example.com")
	}
}

func TestSearchBuildsRequestAndParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/v1/search")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("authorization header = %q", got)
		}
		if got := r.URL.Query().Get("full"); got != "Executor" {
			t.Fatalf("full query = %q", got)
		}
		if got := r.URL.Query()["projects"]; len(got) != 2 || got[0] != "jenkins" || got[1] != "core" {
			t.Fatalf("projects query = %v", got)
		}
		if got := r.URL.Query().Get("maxresults"); got != "10" {
			t.Fatalf("maxresults = %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"resultCount": 1,
			"startDocument": 0,
			"endDocument": 1,
			"time": 12,
			"results": {
				"/src/main.go": [
					{"line":"func <b>Executor</b>() {}", "lineNumber":"42", "tag":"method"}
				]
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/", "secret-token")
	resp, err := client.Search(SearchParams{
		Query:      "Executor",
		Projects:   []string{"jenkins", "core"},
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if resp.ResultCount != 1 {
		t.Fatalf("ResultCount = %d, want 1", resp.ResultCount)
	}
	if resp.DurationMs != 12 {
		t.Fatalf("DurationMs = %d, want 12", resp.DurationMs)
	}

	hits := resp.Results["/src/main.go"]
	if len(hits) != 1 {
		t.Fatalf("len(hits) = %d, want 1", len(hits))
	}
	if hits[0].LineNumber != 42 {
		t.Fatalf("LineNumber = %d, want 42", hits[0].LineNumber)
	}
	if hits[0].Tag != "method" {
		t.Fatalf("Tag = %q, want %q", hits[0].Tag, "method")
	}
}

func TestSearchRejectsEmptyParams(t *testing.T) {
	client := NewClient("http://example.com", "")

	_, err := client.Search(SearchParams{})
	if err == nil {
		t.Fatal("Search error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "search requires query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetFileContentBuildsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/file/content" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/v1/file/content")
		}
		if got := r.URL.Query().Get("path"); got != "/jenkins/src/main.go" {
			t.Fatalf("path = %q", got)
		}

		_, _ = w.Write([]byte("package main\n"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	content, err := client.GetFileContent("jenkins", "/src/main.go")
	if err != nil {
		t.Fatalf("GetFileContent returned error: %v", err)
	}
	if content != "package main\n" {
		t.Fatalf("content = %q", content)
	}
}

func TestBuildSourceRootPath(t *testing.T) {
	testCases := []struct {
		name    string
		project string
		path    string
		want    string
	}{
		{
			name:    "joins project and relative path",
			project: "jenkins",
			path:    "src/main.go",
			want:    "/jenkins/src/main.go",
		},
		{
			name:    "joins project and absolute project path",
			project: "jenkins",
			path:    "/src/main.go",
			want:    "/jenkins/src/main.go",
		},
		{
			name:    "keeps source root path",
			project: "jenkins",
			path:    "/jenkins/src/main.go",
			want:    "/jenkins/src/main.go",
		},
		{
			name:    "handles empty path",
			project: "jenkins",
			path:    "",
			want:    "/jenkins",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildSourceRootPath(tc.project, tc.path); got != tc.want {
				t.Fatalf("buildSourceRootPath(%q, %q) = %q, want %q", tc.project, tc.path, got, tc.want)
			}
		})
	}
}

func TestListProjectsParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/v1/projects")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`["jenkins","workflow"]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	projects, err := client.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects returned error: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("len(projects) = %d, want 2", len(projects))
	}
	if projects[0].Name != "jenkins" || projects[1].Name != "workflow" {
		t.Fatalf("projects = %#v", projects)
	}
}

func TestListProjectsReturnsServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	_, err := client.ListProjects()
	if err == nil {
		t.Fatal("ListProjects error = nil, want error")
	}
	if !strings.Contains(err.Error(), "server returned 502") {
		t.Fatalf("unexpected error: %v", err)
	}
}
