package commands

import "testing"

func TestParseAliases(t *testing.T) {
	p := Parse("n", nil)
	if p.Verb != "north" {
		t.Fatalf("got %q", p.Verb)
	}
	p = Parse("l rusty lamp", nil)
	if p.Verb != "look" || p.Rest != "rusty lamp" {
		t.Fatalf("%q %q", p.Verb, p.Rest)
	}
	p = Parse("'hello there", nil)
	if p.Verb != "say" || p.Rest != "hello there" {
		t.Fatalf("%q %q", p.Verb, p.Rest)
	}
}

func TestParsePackVerb(t *testing.T) {
	canon := func(s string) string {
		if s == "hunt" {
			return "kill"
		}
		return s
	}
	p := Parse("hunt rat", canon)
	if p.Verb != "kill" {
		t.Fatalf("got %q", p.Verb)
	}
	p = Parse("stats", nil)
	if p.Verb != "score" {
		t.Fatalf("stats -> %q", p.Verb)
	}
	p = Parse("light lamp", nil)
	if p.Verb != "use" || p.Rest != "lamp" {
		t.Fatalf("light lamp -> %q %q", p.Verb, p.Rest)
	}
}
