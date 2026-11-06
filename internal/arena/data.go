// Package arena loads and saves arena definition files (Arenas/<world>.yml).
//
// The YAML keys are identical to the PocketMine version, so existing arena files load unchanged.
package arena

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// StringList decodes a YAML list or an index-keyed map into a slice. PHP writes a map when an
// element is removed from the middle of a list.
type StringList []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			var v string
			if err := c.Decode(&v); err != nil {
				return err
			}
			out = append(out, v)
		}
		*s = out
	case yaml.MappingNode:
		out := make([]string, 0, len(n.Content)/2)
		for i := 1; i < len(n.Content); i += 2 {
			var v string
			if err := n.Content[i].Decode(&v); err != nil {
				return err
			}
			out = append(out, v)
		}
		*s = out
	default:
		*s = nil
	}
	return nil
}

// Map decodes a YAML map, or an empty list ("[]"), which PHP emits for an empty array.
type Map[T any] map[string]T

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *Map[T]) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.MappingNode:
		var out map[string]T
		if err := n.Decode(&out); err != nil {
			return err
		}
		*m = out
	case yaml.SequenceNode:
		out := map[string]T{}
		for i, c := range n.Content {
			var v T
			if err := c.Decode(&v); err != nil {
				return err
			}
			out[strconv.Itoa(i)] = v
		}
		*m = out
	default:
		*m = Map[T]{}
	}
	return nil
}

// Data is the persisted definition of one arena. Optional values are pointers so that "unset" is
// distinguishable from zero.
type Data struct {
	DisplayName   *string                   `yaml:"display-name"`
	MaxInTeam     *int                      `yaml:"maxInTeam"`
	WaitingSpawn  *string                   `yaml:"waitingSpawn"`
	World         *string                   `yaml:"world"`
	TeamName      StringList                `yaml:"teamName"`
	TeamColor     Map[string]               `yaml:"teamColor"`
	TeamBed       Map[string]               `yaml:"teamBed"`
	TeamSpawn     Map[string]               `yaml:"teamSpawn"`
	TeamShop      Map[string]               `yaml:"teamShop"`
	TeamUpgrade   Map[string]               `yaml:"teamUpgrade"`
	TeamGenerator Map[Map[string]]          `yaml:"teamGenerator"`
	Generator     Map[StringList]           `yaml:"generator"`
	Blocks        []any                     `yaml:"blocks"`
	DragonPos1    *string                   `yaml:"dragonPos1,omitempty"`
	DragonPos2    *string                   `yaml:"dragonPos2,omitempty"`
	Enabled       bool                      `yaml:"enabled"`
}

// New returns an empty arena definition ready for setup.
func New() *Data {
	return &Data{
		TeamColor:     Map[string]{},
		TeamBed:       Map[string]{},
		TeamSpawn:     Map[string]{},
		TeamShop:      Map[string]{},
		TeamUpgrade:   Map[string]{},
		TeamGenerator: Map[Map[string]]{},
		Generator:     Map[StringList]{},
	}
}

// normalize replaces nil maps with empty ones after decoding.
func (d *Data) normalize() {
	if d.TeamColor == nil {
		d.TeamColor = Map[string]{}
	}
	if d.TeamBed == nil {
		d.TeamBed = Map[string]{}
	}
	if d.TeamSpawn == nil {
		d.TeamSpawn = Map[string]{}
	}
	if d.TeamShop == nil {
		d.TeamShop = Map[string]{}
	}
	if d.TeamUpgrade == nil {
		d.TeamUpgrade = Map[string]{}
	}
	if d.TeamGenerator == nil {
		d.TeamGenerator = Map[Map[string]]{}
	}
	if d.Generator == nil {
		d.Generator = Map[StringList]{}
	}
}

// Dir returns the folder holding arena files.
func Dir(dataDir string) string { return filepath.Join(dataDir, "Arenas") }

// LoadAll loads every arena file in the data directory, keyed by arena (world) name.
func LoadAll(dataDir string) (map[string]*Data, error) {
	files, err := filepath.Glob(filepath.Join(Dir(dataDir), "*.yml"))
	if err != nil {
		return nil, err
	}
	out := map[string]*Data{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		d := New()
		if err := yaml.Unmarshal(b, d); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		d.normalize()
		name := filepath.Base(f)
		out[name[:len(name)-len(".yml")]] = d
	}
	return out, nil
}

// Save writes the arena file for name.
func Save(dataDir, name string, d *Data) error {
	if err := os.MkdirAll(Dir(dataDir), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(d)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(Dir(dataDir), name+".yml"), b, 0o644)
}

// Delete removes the arena file for name.
func Delete(dataDir, name string) {
	_ = os.Remove(filepath.Join(Dir(dataDir), name+".yml"))
}

// Validate reports why the arena cannot be enabled yet, or nil when setup is complete.
func (d *Data) Validate() error {
	if d.World == nil || d.WaitingSpawn == nil || d.DisplayName == nil {
		return errors.New("world or waitingSpawn or display-name cannot be empty")
	}
	if d.MaxInTeam == nil {
		return errors.New("maxInTeam cannot be empty")
	}
	if len(d.TeamName) < 2 || len(d.TeamColor) < 2 || len(d.TeamBed) < 2 || len(d.TeamSpawn) < 2 ||
		len(d.TeamShop) < 2 || len(d.TeamUpgrade) < 2 || len(d.TeamGenerator) < 2 || len(d.Generator) < 1 {
		return errors.New("Team utility cannot be < 2")
	}
	return nil
}

// Max returns the configured players per team, or 0 when unset.
func (d *Data) Max() int {
	if d.MaxInTeam == nil {
		return 0
	}
	return *d.MaxInTeam
}

// Str dereferences an optional string, returning "" for nil.
func Str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
