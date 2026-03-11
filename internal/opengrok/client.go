// Package opengrok provides a client for the OpenGrok REST API.
//
// OpenGrok exposes a REST API under /api/v1/. The three endpoints we care
// about are:
//
//	GET /api/v1/search       – full-text / symbol search
//	GET /api/v1/file/content – raw file content
//	GET /api/v1/projects     – list indexed projects
package opengrok

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client holds the HTTP client and connection details for one OpenGrok instance.
// Create one with NewClient and reuse it; it is safe for concurrent use.
type Client struct {
	baseURL    string       // e.g. "http://localhost:8080/source"
	token      string       // optional Bearer token; empty string means no auth
	httpClient *http.Client
}

// NewClient creates a ready-to-use OpenGrok client.
// baseURL is the root of the OpenGrok web app, e.g. "http://localhost:8080/source".
// token is an optional API token; pass "" if authentication is not required.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

// SearchParams controls what the Search method looks for.
// At least one of Query, Defs, Refs, or Path must be non-empty.
type SearchParams struct {
	// Query is a full-text Lucene query (maps to the ?full= parameter).
	Query string

	// Defs restricts the search to symbol definitions (functions, classes, etc.)
	// and maps to the ?def= parameter.
	Defs string

	// Refs restricts the search to symbol references and maps to ?symbol=.
	Refs string

	// Path is a glob-style file path filter, e.g. "*.go" or "/src/main*".
	// Maps to the ?path= parameter.
	Path string

	// Projects limits the search to specific OpenGrok project names.
	// An empty slice means "search all projects".
	Projects []string

	// MaxResults caps the number of hits returned. 0 means use the server default (25).
	MaxResults int
}

// Hit is one matching line returned by the search API.
type Hit struct {
	// Line is the HTML-formatted source line; OpenGrok wraps matched terms in <b>.
	Line string `json:"line"`

	// LineNumber is the 1-based line number within the file.
	// The OpenGrok API returns this as a JSON string, so we parse it manually.
	LineNumber int

	// Tag is the Lucene document tag (e.g. "method", "class"); may be empty.
	Tag string `json:"tag"`
}

// hitJSON mirrors the raw JSON shape so we can do the string→int conversion.
type hitJSON struct {
	Line       string `json:"line"`
	LineNumber string `json:"lineNumber"`
	Tag        string `json:"tag"`
}

// SearchResponse is the parsed body returned by GET /api/v1/search.
type SearchResponse struct {
	// Results maps each file path (e.g. "/src/main.go") to the hits found in it.
	Results map[string][]Hit

	ResultCount int
	StartDoc    int
	EndDoc      int
	// DurationMs is the server-side query time in milliseconds.
	DurationMs int
}

// searchResponseJSON mirrors the raw JSON so we can post-process the hit list.
type searchResponseJSON struct {
	Results     map[string][]hitJSON `json:"results"`
	ResultCount int                  `json:"resultCount"`
	StartDoc    int                  `json:"startDocument"`
	EndDoc      int                  `json:"endDocument"`
	DurationMs  int                  `json:"time"`
}

// Search queries the OpenGrok /api/v1/search endpoint and returns the results.
func (c *Client) Search(params SearchParams) (*SearchResponse, error) {
	if params.Query == "" && params.Defs == "" && params.Refs == "" && params.Path == "" {
		return nil, fmt.Errorf("opengrok: search requires query, defs, refs, or path")
	}

	q := url.Values{}

	if params.Query != "" {
		q.Set("full", params.Query)
	}
	if params.Defs != "" {
		q.Set("def", params.Defs)
	}
	if params.Refs != "" {
		q.Set("symbol", params.Refs)
	}
	if params.Path != "" {
		q.Set("path", params.Path)
	}
	for _, p := range params.Projects {
		q.Add("projects", p)
	}
	if params.MaxResults > 0 {
		q.Set("maxresults", strconv.Itoa(params.MaxResults))
	}

	body, err := c.get("/api/v1/search?" + q.Encode())
	if err != nil {
		return nil, err
	}

	var raw searchResponseJSON
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("opengrok: decode search response: %w", err)
	}

	// Convert the raw JSON shape into our cleaner SearchResponse type.
	resp := &SearchResponse{
		Results:     make(map[string][]Hit, len(raw.Results)),
		ResultCount: raw.ResultCount,
		StartDoc:    raw.StartDoc,
		EndDoc:      raw.EndDoc,
		DurationMs:  raw.DurationMs,
	}
	for filePath, rawHits := range raw.Results {
		hits := make([]Hit, 0, len(rawHits))
		for _, rh := range rawHits {
			lineNum, _ := strconv.Atoi(rh.LineNumber) // best-effort; 0 if missing
			hits = append(hits, Hit{
				Line:       rh.Line,
				LineNumber: lineNum,
				Tag:        rh.Tag,
			})
		}
		resp.Results[filePath] = hits
	}

	return resp, nil
}

// --------------------------------------------------------------------------
// File content
// --------------------------------------------------------------------------

// GetFileContent fetches the raw source of a single file.
//
// project is the OpenGrok project name (as returned by ListProjects).
// path is the file's path within the project, e.g. "/src/main.go".
//
// OpenGrok returns the file as plain text; we return it as a Go string.
func (c *Client) GetFileContent(project, path string) (string, error) {
	q := url.Values{}
	q.Set("path", buildSourceRootPath(project, path))

	body, err := c.get("/api/v1/file/content?" + q.Encode())
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func buildSourceRootPath(project, path string) string {
	project = strings.Trim(project, "/")
	path = strings.TrimSpace(path)
	if path == "" {
		return "/" + project
	}

	if strings.HasPrefix(path, "/"+project+"/") || path == "/"+project {
		return path
	}

	return "/" + project + "/" + strings.TrimLeft(path, "/")
}

// --------------------------------------------------------------------------
// Projects
// --------------------------------------------------------------------------

// Project is a single code project indexed by OpenGrok.
type Project struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListProjects returns every project currently indexed by OpenGrok.
func (c *Client) ListProjects() ([]Project, error) {
	body, err := c.get("/api/v1/projects")
	if err != nil {
		return nil, err
	}

	// The API returns a plain JSON array of project name strings, e.g. ["proj1","proj2"].
	var names []string
	if err := json.Unmarshal(body, &names); err != nil {
		return nil, fmt.Errorf("opengrok: decode projects response: %w", err)
	}
	projects := make([]Project, len(names))
	for i, n := range names {
		projects[i] = Project{Name: n}
	}
	return projects, nil
}

// --------------------------------------------------------------------------
// Shared HTTP helper
// --------------------------------------------------------------------------

// get performs an authenticated GET to path (relative to baseURL) and returns
// the raw response body. It surfaces non-2xx status codes as errors.
func (c *Client) get(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("opengrok: build request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opengrok: http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("opengrok: read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("opengrok: server returned %d: %s", resp.StatusCode, body)
	}

	return body, nil
}
