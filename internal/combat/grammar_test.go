package combat

import "testing"

func TestThirdPerson(t *testing.T) {
	cases := map[string]string{
		"hit":    "hits",
		"bite":   "bites",
		"slash":  "slashes",
		"lash":   "lashes",
		"bash":   "bashes",
		"punch":  "punches",
		"parry":  "parries",
		"strike": "strikes",
	}
	for in, want := range cases {
		if got := ThirdPerson(in); got != want {
			t.Errorf("%s -> %s want %s", in, got, want)
		}
	}
}
