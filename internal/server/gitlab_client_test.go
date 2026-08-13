package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"metastackr/internal/vcs"
)

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestGitLabClient_UpdateSubmodulePointersOnBranch(t *testing.T) {
	var requestedEndpoints []string
	var receivedBodies []map[string]interface{}

	mockTransport := roundTripFunc(func(r *http.Request) *http.Response {
		requestedEndpoints = append(requestedEndpoints, r.Method+" "+r.URL.Path)

		if strings.HasSuffix(r.URL.Path, "/repository/files/.gitmodules/raw") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`[submodule "services/backend"]
	path = services/backend
	url = git@gitlab.com:group/sub-a.git
[submodule "services/frontend"]
	path = services/frontend
	url = git@gitlab.com:group/sub-b.git
`)),
				Header: make(http.Header),
			}
		}

		if strings.Contains(r.URL.Path, "/repository/branches/main") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"commit":{"id":"head_sha_123"}}`)),
				Header:     make(http.Header),
			}
		}

		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/repository/submodules/") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			receivedBodies = append(receivedBodies, body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"id":"commit_456"}`)),
				Header:     make(http.Header),
			}
		}

		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{"message":"not found"}`)),
			Header:     make(http.Header),
		}
	})

	client := NewGitLabClient("https://gitlab.example.com", "glpat-test-token")
	client.httpClient = &http.Client{Transport: mockTransport}
	updates := []vcs.SubmodulePointerUpdate{
		{
			SubmodulePath: "services/backend",
			SubmoduleRepo: "group/sub-a",
			NewCommitSHA:  "new_sha_a",
		},
		{
			SubmodulePath: "services/frontend",
			SubmoduleRepo: "group/sub-b",
			NewCommitSHA:  "new_sha_b",
		},
	}

	err := client.UpdateSubmodulePointersOnBranch(context.Background(), "group/meta-repo", "feature/test-branch", updates, 0)
	if err != nil {
		t.Fatalf("unexpected error updating submodule pointers: %v", err)
	}

	if len(receivedBodies) != 2 {
		t.Fatalf("expected 2 submodule update requests, got %d", len(receivedBodies))
	}

	for _, body := range receivedBodies {
		if body["branch"] != "feature/test-branch" {
			t.Errorf("expected branch feature/test-branch, got %v", body["branch"])
		}
		if body["commit_sha"] != "head_sha_123" {
			t.Errorf("expected commit_sha head_sha_123, got %v", body["commit_sha"])
		}
	}
}

func TestRefreshGitLabToken(t *testing.T) {
	// Test missing params error handling
	_, _, err := RefreshGitLabToken(context.Background(), "", "", "")
	if err == nil {
		t.Fatal("expected error with empty params")
	}
}
