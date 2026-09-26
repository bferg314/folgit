package github

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// Issue is an open GitHub issue. Pull requests are excluded.
type Issue struct {
	Number    int
	Title     string
	Author    string
	Labels    []string
	Comments  int
	UpdatedAt time.Time
	URL       string
}

// MaxIssues is the most issues fetched for one repo (a single API page).
const MaxIssues = 100

// Issues returns up to MaxIssues open issues for owner/repo, most recently
// updated first. more is true when there are further pages.
func (g *GitHub) Issues(ctx context.Context, owner, repo string) (issues []Issue, more bool, err error) {
	u := fmt.Sprintf("%s/repos/%s/%s/issues?state=open&sort=updated&per_page=%d", g.api,
		url.PathEscape(owner), url.PathEscape(repo), MaxIssues)
	var page []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Comments    int       `json:"comments"`
		UpdatedAt   time.Time `json:"updated_at"`
		HTMLURL     string    `json:"html_url"`
		PullRequest *struct{} `json:"pull_request"`
	}
	next, err := g.get(ctx, u, &page)
	if err != nil {
		return nil, false, err
	}
	for _, it := range page {
		// The issues endpoint also returns pull requests.
		if it.PullRequest != nil {
			continue
		}
		is := Issue{Number: it.Number, Title: it.Title, Author: it.User.Login, Comments: it.Comments,
			UpdatedAt: it.UpdatedAt, URL: it.HTMLURL}
		for _, l := range it.Labels {
			is.Labels = append(is.Labels, l.Name)
		}
		issues = append(issues, is)
	}
	return issues, next != "", nil
}
