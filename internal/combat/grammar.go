package combat

import "strings"

func ThirdPerson(verb string) string {
	v := strings.ToLower(strings.TrimSpace(verb))
	if v == "" {
		return "hits"
	}
	switch {
	case strings.HasSuffix(v, "ss"), strings.HasSuffix(v, "x"), strings.HasSuffix(v, "z"),
		strings.HasSuffix(v, "ch"), strings.HasSuffix(v, "sh"):
		return v + "es"
	case strings.HasSuffix(v, "s"):
		return v
	case len(v) > 1 && strings.HasSuffix(v, "y") && !isVowel(v[len(v)-2]):
		return v[:len(v)-1] + "ies"
	default:
		return v + "s"
	}
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}
