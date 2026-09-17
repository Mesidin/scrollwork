package render

import (
	"strings"

	"sudengine/internal/world"
)

func Automap(w *world.World, player *world.Entity, radius int) string {
	room := w.RoomOf(player)
	if room == nil || !room.HasCoords {
		return "No map."
	}
	if radius < 1 {
		radius = 2
	}
	type key struct{ x, y int }
	seen := map[key]*world.Entity{}
	for _, e := range w.Entities {
		if e.Kind != world.KindRoom || !e.HasCoords {
			continue
		}
		if e.Z != room.Z {
			continue
		}
		if abs(e.X-room.X) > radius || abs(e.Y-room.Y) > radius {
			continue
		}
		seen[key{e.X, e.Y}] = e
	}
	var b strings.Builder
	for y := room.Y + radius; y >= room.Y-radius; y-- {
		var line1, line2 strings.Builder
		for x := room.X - radius; x <= room.X+radius; x++ {
			e := seen[key{x, y}]
			if e == nil {
				line1.WriteString("     ")
				line2.WriteString("     ")
				continue
			}
			cell := " · "
			if e.ID == room.ID {
				cell = " @ "
			} else if e.Name != "" {
				r := []rune(e.Name)
				if len(r) > 3 {
					r = r[:3]
				}
				cell = pad3(string(r))
			}
			h := " "
			if _, ok := e.Exits["west"]; ok {
				h = "─"
			}
			h2 := " "
			if _, ok := e.Exits["east"]; ok {
				h2 = "─"
			}
			line1.WriteString(h + cell + h2)
			v := "     "
			if _, ok := e.Exits["south"]; ok {
				v = "  │  "
			}
			line2.WriteString(v)
		}
		b.WriteString(strings.TrimRight(line1.String(), " "))
		b.WriteByte('\n')
		if y != room.Y-radius {
			b.WriteString(strings.TrimRight(line2.String(), " "))
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func pad3(s string) string {
	r := []rune(s)
	for len(r) < 3 {
		r = append(r, ' ')
	}
	return string(r[:3])
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
