package theme

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func discoverFiles() []string {
	var out []string
	home, _ := os.UserHomeDir()
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" && home != "" {
		state = filepath.Join(home, ".local", "state")
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" && home != "" {
		config = filepath.Join(home, ".config")
	}

	// Omarchy current theme (Linux; same paths work if someone copies the layout).
	if state != "" {
		cur := filepath.Join(state, "omarchy", "current", "theme")
		out = append(out,
			filepath.Join(cur, "colors.toml"),
			filepath.Join(cur, "sudengine.toml"),
		)
	}
	if config != "" {
		out = append(out, filepath.Join(config, "sudengine", "theme.toml"))
	}
	if v := os.Getenv("SUDENGINE_THEME"); v != "" {
		out = append(out, v)
	}
	return out
}

func (p *Palette) applyFiles(paths []string) {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("SUDENGINE_COLOR") == "0" {
		p.NoColor = true
		p.applyHex(nil)
		p.sources = nil
		return
	}
	keys := map[string]string{}
	var seen []watched
	for _, path := range paths {
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		m, err := readTOML(path)
		if err != nil {
			continue
		}
		for k, v := range m {
			keys[k] = v
		}
		if m := strings.ToLower(strings.TrimSpace(m["mode"])); m == "light" || m == "dark" {
			p.Mode = m
		}
		seen = append(seen, watched{path: path, mtime: st.ModTime()})
	}
	if len(keys) > 0 {
		p.applyHex(overlaySemantics(keys))
	}
	p.sources = seen
}

func readTOML(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := map[string]any{}
	if err := toml.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := map[string]string{}
	flatten(out, "", raw)
	return out, nil
}

func flatten(dst map[string]string, prefix string, raw map[string]any) {
	for k, v := range raw {
		key := strings.ToLower(k)
		if prefix != "" {
			key = prefix + "." + key
		}
		switch t := v.(type) {
		case string:
			dst[key] = strings.TrimSpace(t)
		case map[string]any:
			flatten(dst, key, t)
		}
	}
}

// overlaySemantics maps Omarchy colors.toml keys onto our style roles.
func overlaySemantics(in map[string]string) map[string]string {
	get := func(names ...string) string {
		for _, n := range names {
			if v := strings.TrimSpace(in[n]); v != "" {
				return normalizeColor(v)
			}
		}
		return ""
	}
	out := map[string]string{}
	copyIf := func(k, v string) {
		if v != "" {
			out[k] = v
		}
	}
	accent := get("accent", "color6", "color4")
	fg := get("bright_foreground", "foreground", "color7")
	muted := get("muted", "dark_foreground", "color8")
	red := get("red", "color1")
	brightRed := get("color9", "red", "color1")
	yellow := get("yellow", "color3")
	magenta := get("color5")
	cyan := get("color6")
	bg := get("background", "color0")

	copyIf("accent", accent)
	copyIf("room", first(get("room"), accent, cyan))
	copyIf("alert", first(get("alert"), brightRed, yellow, red))
	copyIf("combat", first(get("combat"), red))
	copyIf("say", first(get("say"), magenta, accent))
	copyIf("system", first(get("system"), muted))
	copyIf("build", first(get("build"), yellow))
	copyIf("title", first(get("title"), accent, fg))
	copyIf("muted", muted)
	copyIf("cursor", first(get("cursor", "selection"), accent))
	copyIf("error", first(get("error"), red))
	copyIf("banner", first(get("banner"), accent, magenta))
	copyIf("border", first(get("border"), muted))
	copyIf("status_fg", first(get("status_fg", "background"), bg, "0"))
	copyIf("status_bg", first(get("status_bg"), accent, cyan))

	// explicit sudengine.toml keys win (already in `in` under the same names)
	for _, k := range []string{
		"room", "alert", "combat", "say", "system", "build", "title", "accent",
		"muted", "cursor", "error", "banner", "border", "status_fg", "status_bg",
	} {
		if v := get(k); v != "" {
			out[k] = v
		}
	}
	return out
}

func normalizeColor(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "#") || isANSIIndex(s) {
		return s
	}
	// Omarchy sometimes stores hex without '#'
	if len(s) == 6 && isHex(s) {
		return "#" + s
	}
	return s
}

func isANSIIndex(s string) bool {
	if len(s) == 0 || len(s) > 3 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func first(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func anyChanged(list []watched) bool {
	for _, w := range list {
		st, err := os.Stat(w.path)
		if err != nil {
			return true
		}
		if !st.ModTime().Equal(w.mtime) {
			return true
		}
	}
	// also pick up a theme that appeared after launch
	for _, path := range discoverFiles() {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		found := false
		for _, w := range list {
			if w.path == path {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}
