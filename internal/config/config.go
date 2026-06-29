// Package config manages gitcollect's local state directory, ~/.gitcollect:
// the authentication token store, and the base paths collections and audit
// logs are kept under.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultHost is used when no --host flag is given.
const DefaultHost = "github.com"

// ErrNotAuthenticated is returned by LoadToken when no token has been
// stored for the requested host.
var ErrNotAuthenticated = errors.New("not authenticated")

func homeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return home, nil
}

// Dir returns the absolute path to ~/.gitcollect.
func Dir() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gitcollect"), nil
}

// CollectionsDir returns ~/.gitcollect/collections, where each collection's
// YAML manifest is stored as <name>.yaml.
func CollectionsDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "collections"), nil
}

// AuditDir returns ~/.gitcollect/audit, where each collection's audit log
// is stored as <name>.log.
func AuditDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "audit"), nil
}

func configPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config"), nil
}

// EnsureDir creates dir, and any missing parents, with 0700 permissions.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("could not create directory %s: %w", dir, err)
	}
	return nil
}

// Config holds gitcollect's per-host authentication tokens, plus the
// username each token resolved to the last time it was verified (cached
// during "gitcollect auth" so commands like "list" can reason about
// ownership/membership without a network call).
type Config struct {
	Tokens map[string]string `yaml:"tokens"`
	Users  map[string]string `yaml:"users"`
}

// Load reads ~/.gitcollect/config. A missing file is not an error: it
// returns an empty Config so a first "gitcollect auth" run works cleanly.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Tokens: map[string]string{}, Users: map[string]string{}}, nil
		}
		return nil, fmt.Errorf("could not read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse config: %w", err)
	}
	if cfg.Tokens == nil {
		cfg.Tokens = map[string]string{}
	}
	if cfg.Users == nil {
		cfg.Users = map[string]string{}
	}
	return &cfg, nil
}

// save writes cfg atomically (temp file + rename), enforcing 0600.
func (c *Config) save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := EnsureDir(dir); err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("could not encode config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("could not create temp config file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not write config: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("could not set config permissions: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("could not save config: %w", err)
	}
	return nil
}

// SaveToken stores token for host, overwriting any existing entry, and
// persists the config file.
func SaveToken(host, token string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.Tokens[host] = token
	return cfg.save()
}

// LoadToken returns the stored token for host, or ErrNotAuthenticated if
// none has been saved.
func LoadToken(host string) (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}
	token, ok := cfg.Tokens[host]
	if !ok || token == "" {
		return "", ErrNotAuthenticated
	}
	return token, nil
}

// SaveUser caches the username a host's token resolved to, so commands
// that only need "who am I on this host" don't need a network call to find
// out. Call this right after a token is verified (e.g. during "auth").
func SaveUser(host, username string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.Users[host] = username
	return cfg.save()
}

// LoadUser returns the cached username for host, or "" if none is cached
// (e.g. no token has ever been verified for that host).
func LoadUser(host string) (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}
	return cfg.Users[host], nil
}

// Hosts returns every host with a stored token.
func Hosts() ([]string, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0, len(cfg.Tokens))
	for host := range cfg.Tokens {
		hosts = append(hosts, host)
	}
	return hosts, nil
}
