// Package docs ships engine manuals (embedded) and merges them with pack help.
package docs

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed engine/*.md banner.txt
var embedded embed.FS

const (
	SourceEngine = "engine"
	SourceGame   = "game"
)

type Topic struct {
	Key    string
	Title  string
	Source string
	Body   string
}

func EngineBanner() string {
	b, err := embedded.ReadFile("banner.txt")
	if err != nil {
		return "SUDENGINE"
	}
	return strings.TrimRight(string(b), "\n")
}

func EngineTopics() []Topic {
	var out []Topic
	_ = fs.WalkDir(embedded, "engine", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		b, err := embedded.ReadFile(path)
		if err != nil {
			return nil
		}
		key := strings.TrimSuffix(d.Name(), ".md")
		body := string(b)
		out = append(out, Topic{
			Key:    key,
			Title:  Title(body, key),
			Source: SourceEngine,
			Body:   body,
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func EngineLookup(key string) (Topic, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, t := range EngineTopics() {
		if t.Key == key {
			return t, true
		}
	}
	return Topic{}, false
}

func Title(body, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	if fallback == "" {
		return "untitled"
	}
	return fallback
}

// Merge lists pack topics first, then every engine topic.
// A pack file with the same key is what `help <key>` opens. The engine copy
// stays in the list so building and the other shared manuals remain reachable.
func Merge(packHelp map[string]string) []Topic {
	var out []Topic
	var keys []string
	for k := range packHelp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		body := packHelp[k]
		out = append(out, Topic{
			Key:    k,
			Title:  Title(body, k),
			Source: SourceGame,
			Body:   body,
		})
	}
	out = append(out, EngineTopics()...)
	return out
}

func Lookup(packHelp map[string]string, key string) (Topic, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return Topic{}, false
	}
	if packHelp != nil {
		if body, ok := packHelp[key]; ok {
			return Topic{Key: key, Title: Title(body, key), Source: SourceGame, Body: body}, true
		}
	}
	return EngineLookup(key)
}
