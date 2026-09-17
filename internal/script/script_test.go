package script

import "testing"

func TestSandboxBlocksOS(t *testing.T) {
	err := SandboxForbidden(`os.execute("echo hi")`)
	if err == nil {
		t.Fatal("expected os to be unavailable")
	}
}

func TestSandboxAllowsMath(t *testing.T) {
	err := SandboxForbidden(`assert(math.abs(-2) == 2)`)
	if err != nil {
		t.Fatal(err)
	}
}
