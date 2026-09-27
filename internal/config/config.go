// Package config loads and validates the pagotask YAML configuration.
package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Default is the configuration written on first run.
//
//go:embed default.yaml
var Default []byte

// Config is the whole config.yaml.
type Config struct {
	Hotkey      string            `yaml:"hotkey"`
	DefaultList string            `yaml:"default_list"`
	Lists       map[string]string `yaml:"lists"` // key letter -> Google Tasks list title
	Tags        map[string]Tag    `yaml:"tags"`  // tag id -> tag
	Sources     map[string]Source `yaml:"sources"`
	Rules       []Rule            `yaml:"rules"`
	Google      Google            `yaml:"google"`
}

// Tag is one emoji template.
type Tag struct {
	Emoji string `yaml:"emoji"`
	Key   string `yaml:"key"`             // single letter typed first in the title line
	List  string `yaml:"list,omitempty"`  // list key; empty means default_list
	Title string `yaml:"title,omitempty"` // optional title pattern, e.g. "{who} about {what}"
}

// Source says what to read from a foreground application, keyed by process name.
type Source struct {
	Read string `yaml:"read"` // "url", "title" or "none"
}

// Rule turns a captured URL or title into a prefilled task.
type Rule struct {
	Match string            `yaml:"match"`           // regexp against the URL (or title when On is "title")
	On    string            `yaml:"on,omitempty"`    // "url" (default) or "title"
	Tag   string            `yaml:"tag"`             // tag id
	Title string            `yaml:"title,omitempty"` // template with {1}, {2}... capture groups
	List  map[string]string `yaml:"list,omitempty"`  // regexp on the match -> list key override

	re     *regexp.Regexp
	listRe []listOverride
}

type listOverride struct {
	re   *regexp.Regexp
	list string
}

// Google holds optional bring-your-own OAuth client credentials.
type Google struct {
	ClientID     string `yaml:"client_id,omitempty"`
	ClientSecret string `yaml:"client_secret,omitempty"`
}

// Dir returns the per-user config directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "pagotask")
	return dir, os.MkdirAll(dir, 0o700)
}

// Path returns the config.yaml location.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads config.yaml, writing the default file first if it does not exist.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, Default, 0o600); err != nil {
			return nil, err
		}
		data = Default
	} else if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes and validates YAML config bytes.
func Parse(data []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.Hotkey == "" {
		c.Hotkey = "Win+Shift+T"
	}
	if len(c.Lists) == 0 {
		return errors.New("lists must not be empty")
	}
	for k := range c.Lists {
		if len([]rune(k)) != 1 {
			return fmt.Errorf("list key %q must be a single character", k)
		}
	}
	if c.DefaultList == "" {
		return errors.New("default_list is required")
	}
	if _, ok := c.Lists[c.DefaultList]; !ok {
		return fmt.Errorf("default_list %q is not in lists", c.DefaultList)
	}
	seenKeys := map[string]string{}
	for id, t := range c.Tags {
		if t.Emoji == "" {
			return fmt.Errorf("tag %q has no emoji", id)
		}
		if len([]rune(t.Key)) != 1 {
			return fmt.Errorf("tag %q key %q must be a single character", id, t.Key)
		}
		if other, dup := seenKeys[t.Key]; dup {
			return fmt.Errorf("tags %q and %q share key %q", other, id, t.Key)
		}
		seenKeys[t.Key] = id
		if t.List != "" {
			if _, ok := c.Lists[t.List]; !ok {
				return fmt.Errorf("tag %q refers to unknown list %q", id, t.List)
			}
		}
	}
	for name, s := range c.Sources {
		switch s.Read {
		case "url", "title", "none":
		default:
			return fmt.Errorf("source %q: read must be url, title or none", name)
		}
	}
	for i := range c.Rules {
		r := &c.Rules[i]
		if r.On == "" {
			r.On = "url"
		}
		if r.On != "url" && r.On != "title" {
			return fmt.Errorf("rule %d: on must be url or title", i+1)
		}
		if _, ok := c.Tags[r.Tag]; !ok {
			return fmt.Errorf("rule %d refers to unknown tag %q", i+1, r.Tag)
		}
		re, err := regexp.Compile(r.Match)
		if err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
		r.re = re
		keys := make([]string, 0, len(r.List))
		for k := range r.List {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic override order
		for _, k := range keys {
			lre, err := regexp.Compile(k)
			if err != nil {
				return fmt.Errorf("rule %d list override %q: %w", i+1, k, err)
			}
			if _, ok := c.Lists[r.List[k]]; !ok {
				return fmt.Errorf("rule %d list override %q refers to unknown list %q", i+1, k, r.List[k])
			}
			r.listRe = append(r.listRe, listOverride{re: lre, list: r.List[k]})
		}
	}
	return nil
}

// Regexp returns the compiled match expression of a validated rule.
func (r *Rule) Regexp() *regexp.Regexp { return r.re }

// ListFor returns the list key override for the matched text, or "".
func (r *Rule) ListFor(matched string) string {
	for _, o := range r.listRe {
		if o.re.MatchString(matched) {
			return o.list
		}
	}
	return ""
}

// TagByKey finds the tag whose key letter is k.
func (c *Config) TagByKey(k string) (string, Tag, bool) {
	for id, t := range c.Tags {
		if strings.EqualFold(t.Key, k) {
			return id, t, true
		}
	}
	return "", Tag{}, false
}
