package commands

import (
	"strings"
	"sudengine/internal/world"
)

type Parsed struct {
	Raw  string
	Verb string
	Args []string
	Rest string
}

var engineAliases = map[string]string{
	"l": "look", "look": "look",
	"i": "inventory", "inv": "inventory", "inventory": "inventory",
	"n": "north", "s": "south", "e": "east", "w": "west",
	"u": "up", "d": "down",
	"ne": "northeast", "nw": "northwest", "se": "southeast", "sw": "southwest",
	"get": "get", "take": "get",
	"drop": "drop",
	"put":  "put",
	"give": "give",
	"kill": "kill", "k": "kill", "attack": "kill",
	"flee": "flee",
	"say":  "say", "'": "say",
	"emote": "emote", ":": "emote",
	"ask":  "ask",
	"open": "open", "close": "close",
	"lock": "lock", "unlock": "unlock",
	"wear": "wear", "remove": "remove",
	"use":   "use",
	"exits": "exits",
	"score": "score", "sc": "score", "stats": "score", "stat": "score",
	"help": "help", "manual": "help",
	"light": "use",
	"save":  "save",
	"quit":  "quit", "exit": "quit",
	"examine": "examine", "exa": "examine", "ex": "examine",
	"read":      "examine",
	"go":        "go",
	"equipment": "equipment", "eq": "equipment",
	"who":  "who",
	"time": "time",
}

func Parse(line string, packCanonical func(string) string) Parsed {
	line = strings.TrimSpace(line)
	if line == "" {
		return Parsed{}
	}
	// leading quote / emote
	if strings.HasPrefix(line, "'") {
		return Parsed{Raw: line, Verb: "say", Rest: strings.TrimSpace(line[1:]), Args: strings.Fields(strings.TrimSpace(line[1:]))}
	}
	if strings.HasPrefix(line, ":") {
		return Parsed{Raw: line, Verb: "emote", Rest: strings.TrimSpace(line[1:]), Args: strings.Fields(strings.TrimSpace(line[1:]))}
	}
	fields := strings.Fields(line)
	verb := strings.ToLower(fields[0])
	if packCanonical != nil {
		verb = packCanonical(verb)
	}
	if a, ok := engineAliases[verb]; ok {
		verb = a
	}
	if world.IsDir(verb) {
		verb = world.CanonicalDir(verb)
	}
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(line[len(fields[0]):])
	}
	return Parsed{Raw: line, Verb: verb, Args: fields[1:], Rest: rest}
}

func DefaultAliases() map[string]string {
	return engineAliases
}
