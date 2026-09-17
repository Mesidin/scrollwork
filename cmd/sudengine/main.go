package main

import (
	"fmt"
	"os"
	"path/filepath"

	"sudengine/internal/client"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"

	tea "charm.land/bubbletea/v2"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	gamesDir := pack.FindGamesDir()
	if len(args) == 0 {
		m := client.New(gamesDir)
		p := tea.NewProgram(m)
		_, err := p.Run()
		return err
	}
	switch args[0] {
	case "play", "build":
		if len(args) < 2 {
			return fmt.Errorf("usage: sudengine %s <pack-id>", args[0])
		}
		mode := protocol.ModePlay
		if args[0] == "build" {
			mode = protocol.ModeBuild
		}
		m, err := client.DirectPlay(gamesDir, resolvePack(gamesDir, args[1]), mode)
		if err != nil {
			return err
		}
		_, err = tea.NewProgram(m).Run()
		return err
	case "new":
		if len(args) < 2 {
			return fmt.Errorf("usage: sudengine new <id>")
		}
		id := args[1]
		dir := filepath.Join(gamesDir, id)
		p := pack.Blank(id, dir)
		if err := p.Write(); err != nil {
			return err
		}
		fmt.Printf("Created pack %s at %s\n", p.Meta.ID, dir)
		fmt.Printf("Build it with: sudengine build %s\n", p.Meta.ID)
		return nil
	case "validate":
		if len(args) < 2 {
			return fmt.Errorf("usage: sudengine validate <pack>")
		}
		dir := resolvePack(gamesDir, args[1])
		p, err := pack.Load(dir)
		if err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		fmt.Printf("OK  %s  (%s)\n", p.Meta.ID, p.Dir)
		return nil
	case "install":
		if len(args) < 2 {
			return fmt.Errorf("usage: sudengine install <folder-or-zip>")
		}
		info, err := pack.Install(args[1], gamesDir)
		if err != nil {
			return err
		}
		fmt.Printf("Installed %s (%s) -> %s\n", info.Title, info.ID, info.Dir)
		return nil
	case "help", "-h", "--help":
		fmt.Print(`Erickson Stories / sudengine — a single-player MUD engine

Usage:
  sudengine                 Launcher (pick a pack)
  sudengine play <pack>     Play a pack
  sudengine build <pack>    Build/edit a pack in-engine
  sudengine new <id>        Create a blank pack
  sudengine validate <pack> Check a pack's YAML
  sudengine install <src>   Copy a folder or zip into games/

Packs live in ./games/<id>/  (YAML + Lua, no recompile).
Saves live in ./saves/<pack-id>/.
`)
		return nil
	default:
		return fmt.Errorf("unknown command %q (try sudengine help)", args[0])
	}
}

func resolvePack(gamesDir, id string) string {
	if st, err := os.Stat(id); err == nil && st.IsDir() {
		return id
	}
	p := filepath.Join(gamesDir, id)
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p
	}
	return id
}
