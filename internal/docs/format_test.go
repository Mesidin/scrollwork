package docs

import (
	"strings"
	"testing"
)

func TestFormatHeadingsListsCode(t *testing.T) {
	in := "# Building\n\nWrite **YAML** in `games/id/`.\n\n## Items\n\n- get lamp\n- use lamp\n\n```\nuse:\n  toggle_flag: light\n```\n"
	got := Format(in)
	if strings.Contains(got, "#") {
		t.Fatalf("leftover heading mark:\n%s", got)
	}
	if strings.Contains(got, "**") || strings.Contains(got, "`games") {
		t.Fatalf("leftover emphasis:\n%s", got)
	}
	if !strings.Contains(got, "BUILDING") {
		t.Fatalf("missing title:\n%s", got)
	}
	if !strings.Contains(got, "• get lamp") {
		t.Fatalf("missing bullet:\n%s", got)
	}
	if !strings.Contains(got, "toggle_flag") {
		t.Fatalf("missing code:\n%s", got)
	}
	if strings.Contains(got, "```") {
		t.Fatalf("fence leaked:\n%s", got)
	}
}
