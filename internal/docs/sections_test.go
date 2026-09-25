package docs

import (
	"strings"
	"testing"
)

func TestSectionsSplitOnH2(t *testing.T) {
	topic, ok := EngineLookup("building")
	if !ok {
		t.Fatal("missing building")
	}
	secs := Sections(topic.Body)
	if len(secs) < 8 {
		t.Fatalf("expected building pages, got %d", len(secs))
	}
	if secs[0].Title != "Overview" {
		t.Fatalf("first page %q", secs[0].Title)
	}
	if !strings.Contains(strings.ToLower(secs[0].Body), "recompile") {
		t.Fatalf("overview lost the intro:\n%s", secs[0].Body)
	}
	found := map[string]bool{}
	for _, s := range secs {
		found[s.Title] = true
		if strings.Contains(s.Body, "\n## ") {
			t.Fatalf("page %q still contains another heading", s.Title)
		}
	}
	for _, title := range []string{"Rooms", "Items", "In-engine commands"} {
		if !found[title] {
			t.Fatalf("missing %q in %v", title, found)
		}
	}
	if MatchSection(secs, "rooms") == 0 {
		t.Fatal("rooms matched overview")
	}
	if secs[MatchSection(secs, "rooms")].Title != "Rooms" {
		t.Fatal(secs[MatchSection(secs, "rooms")])
	}
}

func TestSectionsSinglePage(t *testing.T) {
	secs := Sections("# Grit and Calm\n\nGrit is punishment.\n")
	if len(secs) != 1 {
		t.Fatalf("got %d", len(secs))
	}
	if secs[0].Title != "Overview" {
		t.Fatal(secs[0].Title)
	}
}

func TestFormatWidthClips(t *testing.T) {
	in := "# Building\n\n" + strings.Repeat("word ", 40) + "\n\n```\n" + strings.Repeat("x", 80) + "\n```\n"
	got := FormatWidth(in, 24)
	for i, ln := range strings.Split(got, "\n") {
		if len([]rune(ln)) > 24 && strings.TrimSpace(ln) != "" {
			// Box-drawing and ASCII in these manuals are single width.
			if ansiWidth(ln) > 24 {
				t.Fatalf("line %d width %d: %q", i, ansiWidth(ln), ln)
			}
		}
	}
}

func ansiWidth(s string) int {
	n := 0
	for _, r := range s {
		_ = r
		n++
	}
	return n
}
