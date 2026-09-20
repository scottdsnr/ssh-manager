// Package config stores ssh-manager's own settings in
// ~/.config/ssh-manager/config.json.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config is persisted to ~/.config/ssh-manager/config.json.
type Config struct {
	// ConfigFile is the ssh config read from and written to.
	ConfigFile string `json:"config_file"`
	// Color names the UI accent colour. Empty means the UI default.
	Color string `json:"color"`

	path string
}

func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "ssh-manager")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ssh-manager")
}

func Path() string { return filepath.Join(Dir(), "config.json") }

// Default is the conventional ssh config path.
func Default() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "config")
}

// Load reads the config. ok=false means no config exists yet, which is the
// signal to run first-time setup.
func Load() (*Config, bool, error) {
	p := Path()
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{path: p, ConfigFile: Default()}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	c := &Config{path: p}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, false, err
	}
	if c.ConfigFile == "" {
		c.ConfigFile = Default()
	}
	return c, true, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(b, '\n'), 0o644)
}

// ExpandPath resolves ~ and environment variables, and makes the path absolute.
func ExpandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// Candidates lists ssh config files that exist on this machine, offered as
// suggestions during setup.
func Candidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	paths := []string{filepath.Join(home, ".ssh", "config")}
	if m, err := filepath.Glob(filepath.Join(home, ".ssh", "config.d", "*")); err == nil {
		paths = append(paths, m...)
	}
	paths = append(paths, "/etc/ssh/ssh_config")
	var out []string
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// Keys lists private keys in ~/.ssh, offered as IdentityFile suggestions.
func Keys() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(home, ".ssh"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || strings.HasSuffix(n, ".pub") || n == "config" || n == "known_hosts" || n == "authorized_keys" {
			continue
		}
		if strings.HasPrefix(n, "id_") || strings.HasSuffix(n, ".pem") {
			out = append(out, "~/.ssh/"+n)
		}
	}
	return out
}
