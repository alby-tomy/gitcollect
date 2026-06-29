// Package collection implements gitcollect's local manifest: the YAML
// declaration of which members, groups, and repos belong to a collection,
// and who is meant to access what. This package never calls a platform
// API — it only reasons about the local declaration of intent.
package collection

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alby-tomy/gitcollect/internal/config"
)

// Visibility controls whether a collection's existence can be discovered by
// non-members.
type Visibility string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

// CurrentVersion is written into every new collection manifest.
const CurrentVersion = "1"

var (
	ErrNotFound      = errors.New("collection not found")
	ErrAlreadyExists = errors.New("collection already exists")
	ErrInvalidName   = errors.New("invalid name")

	nameRe     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)
	repoNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,100}$`)
	usernameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)
	groupNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,30}$`)
)

// ValidateCollectionName reports whether name is a safe, well-formed
// collection name.
func ValidateCollectionName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w: collection name %q must match %s", ErrInvalidName, name, nameRe.String())
	}
	return nil
}

// ValidateRepoName reports whether name is a safe, well-formed repo name.
func ValidateRepoName(name string) error {
	if !repoNameRe.MatchString(name) || containsUnsafe(name) {
		return fmt.Errorf("%w: repo name %q must match %s", ErrInvalidName, name, repoNameRe.String())
	}
	return nil
}

// ValidateUsername reports whether name is a safe, well-formed username.
func ValidateUsername(name string) error {
	if !usernameRe.MatchString(name) {
		return fmt.Errorf("%w: username %q must match %s", ErrInvalidName, name, usernameRe.String())
	}
	return nil
}

// ValidateGroupName reports whether name is a safe, well-formed group name.
func ValidateGroupName(name string) error {
	if !groupNameRe.MatchString(name) {
		return fmt.Errorf("%w: group name %q must match %s", ErrInvalidName, name, groupNameRe.String())
	}
	return nil
}

func containsUnsafe(s string) bool {
	for _, bad := range []string{"../", "/", "\\", "\x00"} {
		if strings.Contains(s, bad) {
			return true
		}
	}
	return false
}

// RepoAccess defines who can access a single repo within the collection.
// Groups and Users are unioned, not intersected: a caller needs to satisfy
// only one of the two restrictions, if either is set.
type RepoAccess struct {
	Name   string   `yaml:"name"`
	Groups []string `yaml:"groups"`
	Users  []string `yaml:"users"`
}

// Collection is gitcollect's local declaration of intent for a named group
// of repositories: who belongs to it, what groups they form, and which
// repos each group or individual may reach.
type Collection struct {
	Version     string              `yaml:"version"`
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Host        string              `yaml:"host"`
	Owner       string              `yaml:"owner"`
	Visibility  Visibility          `yaml:"visibility"`
	Members     []string            `yaml:"members"`
	Groups      map[string][]string `yaml:"groups"`
	Repos       []RepoAccess        `yaml:"repos"`
	CreatedAt   time.Time           `yaml:"created_at"`
	UpdatedAt   time.Time           `yaml:"updated_at"`

	path string // absolute path on disk; not serialised
}

func manifestPath(name string) (string, error) {
	dir, err := config.CollectionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".yaml"), nil
}

// New creates a fresh, in-memory Collection. Call Save to persist it.
func New(name, host, owner string, visibility Visibility) (*Collection, error) {
	if err := ValidateCollectionName(name); err != nil {
		return nil, err
	}
	path, err := manifestPath(name)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &Collection{
		Version:     CurrentVersion,
		Name:        name,
		Host:        host,
		Owner:       owner,
		Visibility:  visibility,
		Members:     []string{},
		Groups:      map[string][]string{},
		Repos:       []RepoAccess{},
		CreatedAt:   now,
		UpdatedAt:   now,
		path:        path,
	}, nil
}

// Exists reports whether a manifest already exists for name.
func Exists(name string) (bool, error) {
	path, err := manifestPath(name)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Load reads and validates the manifest for name.
func Load(name string) (*Collection, error) {
	path, err := manifestPath(name)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("could not read collection %q: %w", name, err)
	}

	var c Collection
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("could not parse collection %q: %w", name, err)
	}
	c.path = path

	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("collection %q failed validation: %w", name, err)
	}
	return &c, nil
}

// List returns the names of every collection manifest on disk.
func List() ([]string, error) {
	dir, err := config.CollectionsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("could not list collections: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		const ext = ".yaml"
		if len(e.Name()) > len(ext) && e.Name()[len(e.Name())-len(ext):] == ext {
			names = append(names, e.Name()[:len(e.Name())-len(ext)])
		}
	}
	return names, nil
}

// Validate checks structural integrity of the manifest: every group member
// must be a collection member, and every repo's group/user references must
// resolve to real groups and members.
func (c *Collection) Validate() error {
	if err := ValidateCollectionName(c.Name); err != nil {
		return err
	}
	if c.Visibility != VisibilityPublic && c.Visibility != VisibilityPrivate {
		return fmt.Errorf("invalid visibility %q", c.Visibility)
	}

	members := make(map[string]bool, len(c.Members))
	for _, m := range c.Members {
		members[m] = true
	}

	for group, users := range c.Groups {
		if err := ValidateGroupName(group); err != nil {
			return err
		}
		for _, u := range users {
			if !members[u] {
				return fmt.Errorf("group %q references %q, who is not a member", group, u)
			}
		}
	}

	for _, r := range c.Repos {
		if err := ValidateRepoName(r.Name); err != nil {
			return err
		}
		for _, g := range r.Groups {
			if _, ok := c.Groups[g]; !ok {
				return fmt.Errorf("repo %q references unknown group %q", r.Name, g)
			}
		}
		for _, u := range r.Users {
			if !members[u] {
				return fmt.Errorf("repo %q references %q, who is not a member", r.Name, u)
			}
		}
	}
	return nil
}

// Save validates the manifest and writes it atomically (temp file +
// rename) at 0600.
func (c *Collection) Save() error {
	if err := c.Validate(); err != nil {
		return err
	}

	dir, err := config.CollectionsDir()
	if err != nil {
		return err
	}
	if err := config.EnsureDir(dir); err != nil {
		return err
	}

	if c.path == "" {
		path, err := manifestPath(c.Name)
		if err != nil {
			return err
		}
		c.path = path
	}

	c.UpdatedAt = time.Now().UTC()

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("could not encode collection %q: %w", c.Name, err)
	}

	tmp, err := os.CreateTemp(dir, c.Name+"-*.tmp")
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write collection %q: %w", c.Name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not write collection %q: %w", c.Name, err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("could not set permissions on collection %q: %w", c.Name, err)
	}
	if err := os.Rename(tmpPath, c.path); err != nil {
		return fmt.Errorf("could not save collection %q: %w", c.Name, err)
	}
	return nil
}

// Delete removes the collection's manifest from disk.
func (c *Collection) Delete() error {
	if c.path == "" {
		path, err := manifestPath(c.Name)
		if err != nil {
			return err
		}
		c.path = path
	}
	if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not delete collection %q: %w", c.Name, err)
	}
	return nil
}
