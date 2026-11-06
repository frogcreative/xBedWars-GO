// Package cfg loads config.yml and exposes dotted-path accessors such as "game.timers.upgrade-
// interval".
package cfg

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed config.yml
// defaultConfig is the bundled config.yml written to disk on first start.
var defaultConfig []byte

// root is the parsed configuration tree.
var root = map[string]any{}

// Load reads config.yml from dataDir, writing the bundled default first when the file does not
// exist.
func Load(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dataDir, "config.yml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, defaultConfig, 0o644); err != nil {
			return err
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("config.yml: %w", err)
	}
	root = m
	return nil
}

// Get returns the value at the dotted path, or def when any path segment is missing.
func Get(key string, def any) any {
	var cur any = root
	for _, part := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return def
		}
		cur, ok = m[part]
		if !ok {
			return def
		}
	}
	return cur
}

// String returns the string at key, or def when it is missing or not a string.
func String(key, def string) string {
	if s, ok := Get(key, def).(string); ok {
		return s
	}
	return def
}

// Int returns the integer at key, accepting any YAML numeric type, or def when it is missing.
func Int(key string, def int) int {
	switch v := Get(key, def).(type) {
	case int:
		return v
	case int64:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	}
	return def
}

// Float returns the number at key as a float64, or def when it is missing.
func Float(key string, def float64) float64 {
	switch v := Get(key, def).(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return def
}

// Strings returns the list of strings at key (for example multi-line chat banners).
func Strings(key string) []string {
	list, ok := Get(key, nil).([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Keys returns the sorted keys of the map at key.
func Keys(key string) []string {
	m, ok := Get(key, nil).(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
