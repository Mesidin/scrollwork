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
	"talk": "talk",
	"list": "list", "lshop": "list",
	"buy":  "buy",
	"sell": "sell",
	"open": "open", "close": "close",
	"lock": "lock", "unlock": "unlock",
	"wear": "wear", "remove": "remove",
	"use":     "use",
	"exits":   "exits",
	"search":  "search",
	"improve": "improve",
	"raise":   "raise",
	"train":   "train",
	"score":   "score", "sc": "score", "stats": "score", "stat": "score",
	"help": "help", "manual": "help",
	"light": "use",
	"save":  "save",
	"quit":  "quit", "exit": "quit",
	"examine": "examine", "exa": "examine", "ex": "examine",
	"read":    "examine",
	"go":      "go",
	"who":     "who",
	"time":    "time",
	"clock":   "time",
	"date":    "time",
	"weather": "weather",
	"sky":     "sky",
	"repair":   "repair",
	"fix":      "repair",
	"craft":    "craft",
	"make":     "craft",
	"recipes":  "recipes", "rlist": "recipes",
	"deposit":  "deposit",
	"withdraw": "withdraw",
	"balance":  "balance", "bank": "balance", "account": "balance",
	"gather":   "gather", "forage": "gather", "harvest": "gather",
	"quests":   "quests", "tasks": "quests", "journal": "quests", "questlog": "quests",
	"quest":    "quest", "task": "quest",
	"skin":     "skin", "butcher": "skin",
	"salvage":  "salvage", "breakdown": "salvage", "dismantle": "salvage", "deconstruct": "salvage",
	"follow":   "follow", "fol": "follow",
	"unfollow": "unfollow", "stopfollow": "unfollow", "unfol": "unfollow",
	"lose":     "lose", "ditch": "lose", "shake": "lose",
	"group":    "group",
	"party":    "party", "gt": "party",
	"tell":     "tell",
	"gossip":   "gossip", "ooc": "gossip",
	"invite":   "invite",
	"join":     "join",
	"order":    "order", "command": "order",
	"feed":     "feed",
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
