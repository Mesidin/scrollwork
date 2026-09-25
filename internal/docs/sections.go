package docs

import (
	"strings"
	"unicode"
)

// Section is one page of a help topic. Headings at level two (`##`) start a
// new page. Text before the first of those is the overview.
type Section struct {
	Key   string
	Title string
	Body  string
}

func Sections(body string) []Section {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	title := "Overview"
	var buf []string
	var out []Section
	flush := func() {
		text := strings.TrimSpace(strings.Join(buf, "\n"))
		if text == "" {
			return
		}
		out = append(out, Section{Key: Slug(title), Title: title, Body: text})
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if isH2(trim) {
			flush()
			buf = nil
			title = strings.TrimSpace(strings.TrimPrefix(trim, "##"))
			buf = append(buf, line)
			continue
		}
		buf = append(buf, line)
	}
	flush()
	if len(out) == 0 {
		return []Section{{Key: "overview", Title: "Overview", Body: strings.TrimSpace(body)}}
	}
	used := map[string]int{}
	for i := range out {
		key := out[i].Key
		if key == "" {
			key = "section"
		}
		n := used[key]
		used[key] = n + 1
		if n > 0 {
			key = key + "-" + itoa(n+1)
		}
		out[i].Key = key
	}
	return out
}

func isH2(trim string) bool {
	return strings.HasPrefix(trim, "## ") && !strings.HasPrefix(trim, "###")
}

// MatchSection finds a page by title or key. An empty query is the first page.
func MatchSection(sections []Section, query string) int {
	q := Slug(query)
	if q == "" || len(sections) == 0 {
		return 0
	}
	for i, s := range sections {
		if s.Key == q || Slug(s.Title) == q {
			return i
		}
	}
	for i, s := range sections {
		if strings.Contains(s.Key, q) || strings.Contains(Slug(s.Title), q) {
			return i
		}
	}
	return 0
}

func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d [8]byte
	i := len(d)
	for n > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}
