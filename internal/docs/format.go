package docs

import (
	"strings"
	"unicode"
)

const wrapWidth = 72

// Format turns author Markdown into text that reads cleanly in a TUI:
// headings as titles, lists as bullets, code fences as indented blocks,
// no leftover # or ** markers.
func Format(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	lines := strings.Split(md, "\n")
	var out []string
	inFence := false
	for i := 0; i < len(lines); i++ {
		raw := strings.TrimRightFunc(lines[i], unicode.IsSpace)
		trim := strings.TrimSpace(raw)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			if inFence {
				out = append(out, "")
			} else if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		if inFence {
			out = append(out, "  "+strings.TrimRight(raw, " "))
			continue
		}
		if trim == "" {
			if len(out) == 0 || out[len(out)-1] == "" {
				continue
			}
			out = append(out, "")
			continue
		}
		if strings.HasPrefix(trim, "#") {
			level := 0
			for level < len(trim) && trim[level] == '#' {
				level++
			}
			title := strings.TrimSpace(strings.TrimLeft(trim, "#"))
			title = inline(title)
			if title == "" {
				continue
			}
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			if level <= 1 {
				out = append(out, strings.ToUpper(title))
				n := len([]rune(title))
				if n > 40 {
					n = 40
				}
				if n < 3 {
					n = 3
				}
				out = append(out, strings.Repeat("─", n))
			} else {
				out = append(out, title)
			}
			out = append(out, "")
			continue
		}
		bullet := ""
		rest := trim
		switch {
		case strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "* "):
			bullet = "  • "
			rest = strings.TrimSpace(trim[2:])
		case len(trim) > 2 && unicode.IsDigit(rune(trim[0])):
			j := 0
			for j < len(trim) && unicode.IsDigit(rune(trim[j])) {
				j++
			}
			if j < len(trim) && (trim[j] == '.' || trim[j] == ')') && j+1 < len(trim) && trim[j+1] == ' ' {
				bullet = "  " + trim[:j+1] + " "
				rest = strings.TrimSpace(trim[j+2:])
			}
		}
		rest = inline(rest)
		if bullet != "" {
			wrapped := wrap(rest, wrapWidth-len([]rune(bullet)))
			if len(wrapped) == 0 {
				out = append(out, strings.TrimRight(bullet, " "))
				continue
			}
			out = append(out, bullet+wrapped[0])
			pad := strings.Repeat(" ", len([]rune(bullet)))
			for _, w := range wrapped[1:] {
				out = append(out, pad+w)
			}
			continue
		}
		out = append(out, wrap(rest, wrapWidth)...)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

func inline(s string) string {
	s = stripWrap(s, "**")
	s = stripWrap(s, "__")
	s = stripWrap(s, "*")
	s = stripWrap(s, "_")
	s = stripWrap(s, "`")
	// [label](url) -> label
	for {
		i := strings.Index(s, "](")
		if i < 0 {
			break
		}
		open := strings.LastIndex(s[:i], "[")
		close := strings.Index(s[i:], ")")
		if open < 0 || close < 0 {
			break
		}
		label := s[open+1 : i]
		s = s[:open] + label + s[i+close+1:]
	}
	return s
}

func stripWrap(s, mark string) string {
	if mark == "" {
		return s
	}
	for {
		a := strings.Index(s, mark)
		if a < 0 {
			return s
		}
		b := strings.Index(s[a+len(mark):], mark)
		if b < 0 {
			return s
		}
		inner := s[a+len(mark) : a+len(mark)+b]
		s = s[:a] + inner + s[a+len(mark)+b+len(mark):]
	}
}

func wrap(s string, width int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if width < 20 {
		width = 20
	}
	words := strings.Fields(s)
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			cur.WriteString(w)
			continue
		}
		if cur.Len()+1+len(w) > width {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
