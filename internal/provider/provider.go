// Package provider defines the interface to git hosting services.
package provider

import (
	"context"
	"strings"
	"time"
)

// Repo is a repository as listed by a hosting provider.
type Repo struct {
	Host        string // "github.com"
	Owner       string
	Name        string
	CloneURL    string // preferred URL (ssh or https, per config)
	WebURL      string
	Description string
	Language    string
	Stars       int
	PushedAt    time.Time
	Private     bool
	Fork        bool
	Archived    bool
}

// FullName is "owner/name".
func (r Repo) FullName() string { return r.Owner + "/" + r.Name }

// Key matches match.Key output for local remotes: "host/owner/name".
func (r Repo) Key() string { return strings.ToLower(r.Host + "/" + r.Owner + "/" + r.Name) }

// Provider lists the repositories a user has access to.
type Provider interface {
	Name() string
	ListRepos(ctx context.Context) ([]Repo, error)
}
