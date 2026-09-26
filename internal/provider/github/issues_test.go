package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testClient(t *testing.T, h http.HandlerFunc) *GitHub {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &GitHub{api: srv.URL, token: "t", client: srv.Client()}
}

func TestIssues(t *testing.T) {
	g := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/issues" || r.URL.Query().Get("state") != "open" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Link", `<https://api.github.com/x?page=2>; rel="next"`)
		w.Write([]byte(`[
			{"number": 7, "title": "Crash on start", "user": {"login": "ada"},
			 "labels": [{"name": "bug"}, {"name": "p1"}], "comments": 3,
			 "updated_at": "2026-09-01T10:00:00Z", "html_url": "https://github.com/o/r/issues/7"},
			{"number": 8, "title": "A pull request", "user": {"login": "bob"},
			 "updated_at": "2026-09-02T10:00:00Z", "pull_request": {"url": "x"}}
		]`))
	})

	issues, more, err := g.Issues(context.Background(), "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Error("expected more=true from Link header")
	}
	if len(issues) != 1 {
		t.Fatalf("pull requests should be filtered out, got %+v", issues)
	}
	is := issues[0]
	if is.Number != 7 || is.Author != "ada" || is.Comments != 3 || len(is.Labels) != 2 || is.URL == "" || is.UpdatedAt.IsZero() {
		t.Errorf("unexpected issue %+v", is)
	}
}

func TestIssuesNotFound(t *testing.T) {
	g := testClient(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if _, _, err := g.Issues(context.Background(), "o", "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
