// Package github implements provider.Provider for github.com.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/bferg314/folgit/internal/config"
	"github.com/bferg314/folgit/internal/provider"
)

// ErrNoToken is returned when no GitHub credentials can be found.
var ErrNoToken = errors.New("no GitHub token: run `gh auth login` or set GITHUB_TOKEN")

// GitHub lists repositories via the REST API.
type GitHub struct {
	api      string // base URL, overridable in tests
	token    string
	protocol string
	opt      config.GitHub
	client   *http.Client
}

// New finds a token (GH_TOKEN, GITHUB_TOKEN, then `gh auth token`) and
// resolves the clone protocol.
func New(ctx context.Context, opt config.GitHub) (*GitHub, error) {
	token := firstNonEmpty(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		token = ghOutput(ctx, "auth", "token")
	}
	if token == "" {
		return nil, ErrNoToken
	}
	protocol := opt.Protocol
	if protocol == "" {
		protocol = ghOutput(ctx, "config", "get", "git_protocol", "-h", "github.com")
	}
	if protocol != "ssh" {
		protocol = "https"
	}
	return &GitHub{
		api:      "https://api.github.com",
		token:    token,
		protocol: protocol,
		opt:      opt,
		client:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (g *GitHub) Name() string { return "GitHub" }

// ListRepos returns owned (and optionally org and starred) repositories,
// most recently pushed first.
func (g *GitHub) ListRepos(ctx context.Context) ([]provider.Repo, error) {
	affiliation := "owner,collaborator"
	if g.opt.IncludeOrgs {
		affiliation += ",organization_member"
	}
	all, err := g.list(ctx, g.api+"/user/repos?per_page=100&sort=pushed&affiliation="+affiliation)
	if err != nil {
		return nil, err
	}
	if g.opt.IncludeStarred {
		starred, err := g.list(ctx, g.api+"/user/starred?per_page=100")
		if err != nil {
			return nil, err
		}
		all = append(all, starred...)
	}

	seen := make(map[string]bool, len(all))
	repos := make([]provider.Repo, 0, len(all))
	for _, r := range all {
		if seen[r.FullName] || (r.Archived && !g.opt.IncludeArchived) || (r.Fork && !g.opt.IncludeForks) {
			continue
		}
		seen[r.FullName] = true
		url := r.CloneURL
		if g.protocol == "ssh" {
			url = r.SSHURL
		}
		repos = append(repos, provider.Repo{
			Host:        "github.com",
			Owner:       r.Owner.Login,
			Name:        r.Name,
			CloneURL:    url,
			WebURL:      r.HTMLURL,
			Description: r.Description,
			Language:    r.Language,
			Stars:       r.Stars,
			PushedAt:    r.PushedAt,
			Private:     r.Private,
			Fork:        r.Fork,
			Archived:    r.Archived,
		})
	}
	return repos, nil
}

type apiRepo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	CloneURL    string    `json:"clone_url"`
	SSHURL      string    `json:"ssh_url"`
	HTMLURL     string    `json:"html_url"`
	Description string    `json:"description"`
	Language    string    `json:"language"`
	Stars       int       `json:"stargazers_count"`
	PushedAt    time.Time `json:"pushed_at"`
	Private     bool      `json:"private"`
	Fork        bool      `json:"fork"`
	Archived    bool      `json:"archived"`
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// list follows Link pagination until the last page.
func (g *GitHub) list(ctx context.Context, url string) ([]apiRepo, error) {
	var all []apiRepo
	for url != "" {
		var page []apiRepo
		next, err := g.get(ctx, url, &page)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		url = next
	}
	return all, nil
}

// ErrNotFound is returned for 404s: the repo doesn't exist, isn't visible
// to this token, or (for issues) has issues turned off.
var ErrNotFound = errors.New("github: not found")

// get fetches one API page into v and returns the next page's URL, if any.
func (g *GitHub) get(ctx context.Context, url string, v any) (next string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return "", ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("github: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return "", fmt.Errorf("github: decoding response: %w", err)
	}
	if m := nextLink.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
		next = m[1]
	}
	return next, nil
}

func ghOutput(ctx context.Context, args ...string) string {
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
