package dice

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
)

var spec = regexp.MustCompile(`^(?:(\d+)d(\d+)|(\d+))([+-]\d+)?$`)

// Roll parses "1d6", "2d4+1", "6", "10-2" and returns a total.
func Roll(rng *rand.Rand, expr string) (int, error) {
	expr = strings.TrimSpace(strings.ToLower(expr))
	if expr == "" {
		return 0, fmt.Errorf("empty dice expression")
	}
	m := spec.FindStringSubmatch(expr)
	if m == nil {
		return 0, fmt.Errorf("bad dice expression %q", expr)
	}
	total := 0
	if m[1] != "" {
		n, _ := strconv.Atoi(m[1])
		sides, _ := strconv.Atoi(m[2])
		if n < 1 || n > 100 || sides < 1 || sides > 1000 {
			return 0, fmt.Errorf("dice out of range %q", expr)
		}
		for i := 0; i < n; i++ {
			total += rng.Intn(sides) + 1
		}
	} else {
		v, _ := strconv.Atoi(m[3])
		total = v
	}
	if m[4] != "" {
		mod, _ := strconv.Atoi(m[4])
		total += mod
	}
	return total, nil
}

// MustRoll is Roll that returns 0 on error.
func MustRoll(rng *rand.Rand, expr string) int {
	n, err := Roll(rng, expr)
	if err != nil {
		return 0
	}
	return n
}
