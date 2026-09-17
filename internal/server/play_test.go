package server

import (
	"path/filepath"
	"strings"
	"testing"

	"sudengine/internal/protocol"
	"sudengine/internal/save"
	"sudengine/internal/world"
)

func drain(s *Server) {
	for {
		select {
		case <-s.Session.Out:
		default:
			return
		}
	}
}

func texts(evs []protocol.Event) string {
	var b strings.Builder
	for _, ev := range evs {
		if t, ok := ev.(protocol.TextEvent); ok {
			b.WriteString(t.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func TestOldHousePlaythrough(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModePlay, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	out := texts(srv.HandleLine("look"))
	if !strings.Contains(out, "Porch") && !strings.Contains(out, "porch") {
		t.Fatalf("expected porch, got:\n%s", out)
	}

	out = texts(srv.HandleLine("n"))
	if !strings.Contains(strings.ToLower(out), "foyer") {
		t.Fatalf("expected foyer:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "maid") {
		t.Fatalf("expected maid:\n%s", out)
	}

	out = texts(srv.HandleLine("ask maid about key"))
	if !strings.Contains(strings.ToLower(out), "parlor") {
		t.Fatalf("maid should mention parlor:\n%s", out)
	}

	_ = srv.HandleLine("n")
	out = texts(srv.HandleLine("get key"))
	if !strings.Contains(strings.ToLower(out), "take") {
		t.Fatalf("get key:\n%s", out)
	}
	_ = srv.HandleLine("get coat")
	out = texts(srv.HandleLine("wear coat"))
	if !strings.Contains(strings.ToLower(out), "wear") {
		t.Fatalf("wear:\n%s", out)
	}

	_ = srv.HandleLine("s")
	_ = srv.HandleLine("e")
	out = texts(srv.HandleLine("get lamp"))
	if !strings.Contains(strings.ToLower(out), "lamp") {
		t.Fatalf("get lamp:\n%s", out)
	}
	out = texts(srv.HandleLine("use lamp"))
	if !strings.Contains(strings.ToLower(out), "light") {
		t.Fatalf("use lamp:\n%s", out)
	}

	_ = srv.HandleLine("w")
	out = texts(srv.HandleLine("unlock down"))
	if !strings.Contains(strings.ToLower(out), "unlock") {
		t.Fatalf("unlock:\n%s", out)
	}
	_ = srv.HandleLine("open down")
	out = texts(srv.HandleLine("d"))
	if !strings.Contains(strings.ToLower(out), "cellar") {
		t.Fatalf("cellar:\n%s", out)
	}

	out = texts(srv.HandleLine("e"))
	if !strings.Contains(strings.ToLower(out), "wine") && !strings.Contains(strings.ToLower(out), "rat") {
		t.Fatalf("wine cellar:\n%s", out)
	}

	_ = srv.HandleLine("kill rat")
	for i := 0; i < 40; i++ {
		srv.tick()
	}
	// rat should be a corpse or gone
	room := srv.World.RoomOf(srv.World.Player())
	foundRat := false
	foundCorpse := false
	for _, e := range srv.World.Children(room.ID) {
		if e.Kind == world.KindMobile && e.HasKeyword("rat") {
			foundRat = true
		}
		if e.HasKeyword("corpse") {
			foundCorpse = true
		}
	}
	if foundRat {
		t.Fatal("rat still alive after many ticks")
	}
	if !foundCorpse {
		t.Fatal("expected a corpse")
	}

	save.Root = t.TempDir()
	if err := srv.ctx().SaveSnap("test"); err != nil {
		t.Fatal(err)
	}

	drain(srv)
	out = texts(srv.HandleLine("help playing"))
	if !strings.Contains(strings.ToLower(out), "single-player") {
		t.Fatalf("engine help playing:\n%s", out)
	}
	if strings.Contains(out, "## ") || strings.Contains(out, "```") {
		t.Fatalf("help still looks like raw markdown:\n%s", out)
	}
	out = texts(srv.HandleLine("help house"))
	if !strings.Contains(strings.ToLower(out), "porch") {
		t.Fatalf("game help house:\n%s", out)
	}
}

func TestChargenRole(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, _, err := Launch(Options{
		PackDir:    dir,
		Mode:       protocol.ModePlay,
		Seed:       1,
		PlayerName: "Ada",
		RoleID:     "reckless",
	})
	if err != nil {
		t.Fatal(err)
	}
	pl := srv.World.Player()
	if pl.Name != "Ada" {
		t.Fatalf("name %s", pl.Name)
	}
	if pl.Res("hp").Max != 30 {
		t.Fatalf("reckless grit max %d", pl.Res("hp").Max)
	}
}

func TestBuildDig(t *testing.T) {
	dir := filepath.Join("..", "..", "games", "old-house")
	srv, _, err := Launch(Options{PackDir: dir, Mode: protocol.ModeBuild, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	out := texts(srv.HandleLine("dig west Garden"))
	if !strings.Contains(out, "Garden") && !strings.Contains(strings.ToLower(out), "dug") {
		t.Fatalf("dig:\n%s", out)
	}
	room := srv.World.RoomOf(srv.World.Player())
	if room == nil || !strings.Contains(strings.ToLower(room.Name), "garden") {
		t.Fatalf("expected to be in garden, got %#v", room)
	}
}
