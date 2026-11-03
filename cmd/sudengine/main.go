package main

import (
	"context"
	"fmt"
	stdnet "net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sudengine/internal/client"
	snet "sudengine/internal/net"
	"sudengine/internal/pack"
	"sudengine/internal/protocol"
	"sudengine/internal/server"

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
	case "serve":
		if len(args) < 2 {
			return fmt.Errorf("usage: sudengine serve <pack> [--port 4000] [--telnet 4023]")
		}
		for _, a := range args[1:] {
			if a == "build" || a == "--build" {
				return fmt.Errorf("build mode is not permitted in server mode")
			}
		}
		packID := ""
		port := 4000
		telnetPort := snet.DefaultTelnetPort
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--port", "-port", "-p":
				if i+1 >= len(args) {
					return fmt.Errorf("--port requires a value")
				}
				i++
				p, err := strconv.Atoi(args[i])
				if err != nil || p < 1 || p > 65535 {
					return fmt.Errorf("invalid port %q", args[i])
				}
				port = p
			case "--telnet", "-telnet":
				if i+1 >= len(args) {
					return fmt.Errorf("--telnet requires a value")
				}
				i++
				tp, err := strconv.Atoi(args[i])
				if err != nil || tp < 0 || tp > 65535 {
					return fmt.Errorf("invalid telnet port %q", args[i])
				}
				telnetPort = tp
			default:
				if packID == "" && !strings.HasPrefix(args[i], "-") {
					packID = args[i]
				}
			}
		}
		if packID == "" {
			return fmt.Errorf("usage: sudengine serve <pack> [--port 4000] [--telnet 4023]")
		}
		dir := resolvePack(gamesDir, packID)
		srv, err := server.LaunchServer(server.Options{
			PackDir: dir,
			Mode:    protocol.ModePlay,
			Persist: true,
		})
		if err != nil {
			return err
		}
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := snet.Listen(addr)
		if err != nil {
			return err
		}
		defer ln.Close()
		var telnetLn stdnet.Listener
		if telnetPort > 0 {
			tAddr := fmt.Sprintf("127.0.0.1:%d", telnetPort)
			tLn, tErr := snet.ListenTelnet(tAddr)
			if tErr != nil {
				return tErr
			}
			telnetLn = tLn
			defer telnetLn.Close()
		}
		var lanListeners []stdnet.Listener
		var lanAddrs []string
		for _, ip := range snet.LocalLANIPs() {
			lAddr := fmt.Sprintf("%s:%d", ip, port)
			lLn, lErr := snet.Listen(lAddr)
			if lErr != nil {
				fmt.Fprintf(os.Stderr, "lan listen %s: %v\n", lAddr, lErr)
				continue
			}
			lanListeners = append(lanListeners, lLn)
			lanAddrs = append(lanAddrs, lLn.Addr().String())
		}
		defer func() {
			for _, lLn := range lanListeners {
				_ = lLn.Close()
			}
		}()

		var extras []string
		if len(lanAddrs) > 0 {
			extras = append(extras, "LAN "+strings.Join(lanAddrs, ", "))
		}
		if telnetLn != nil {
			extras = append(extras, fmt.Sprintf("telnet %s", telnetLn.Addr()))
		}
		if len(extras) > 0 {
			fmt.Printf("Serving %s (%s) on %s (%s)\n", srv.Pack.Meta.Title, srv.Pack.Meta.ID, ln.Addr(), strings.Join(extras, ", "))
		} else {
			fmt.Printf("Serving %s (%s) on %s\n", srv.Pack.Meta.Title, srv.Pack.Meta.ID, ln.Addr())
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if telnetLn != nil {
			go func() {
				_ = snet.ServeTelnet(ctx, srv, telnetLn)
			}()
		}
		for _, lLn := range lanListeners {
			lLn := lLn
			go func() {
				_ = snet.Serve(ctx, srv, lLn)
			}()
		}
		serveErr := make(chan error, 1)
		go func() {
			err := snet.Serve(ctx, srv, ln)
			if err != nil && ctx.Err() == nil {
				cancel()
				serveErr <- err
				return
			}
			serveErr <- nil
		}()
		srv.Run(ctx)
		cancel()
		return <-serveErr
	case "connect":
		if len(args) < 2 {
			return fmt.Errorf("usage: sudengine connect <host:port> [--account <user>] [--name <name>] [--race <race>] [--role <role>]")
		}
		addr := snet.NormalizeClientAddr(args[1])
		account := ""
		name := "you"
		race := ""
		role := ""
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--account", "-account", "-a":
				if i+1 < len(args) {
					i++
					account = args[i]
				}
			case "--password", "-password", "--pass", "-pass":
				return fmt.Errorf("passwords are read from the terminal, not from --password")
			case "--name", "-name", "-n":
				if i+1 < len(args) {
					i++
					name = args[i]
				}
			case "--race", "-race":
				if i+1 < len(args) {
					i++
					race = args[i]
				}
			case "--role", "-role":
				if i+1 < len(args) {
					i++
					role = args[i]
				}
			}
		}
		if account == "" {
			probeCtx, probeCancel := context.WithTimeout(context.Background(), 2*time.Second)
			info, err := snet.Probe(probeCtx, addr)
			probeCancel()
			if err != nil {
				return fmt.Errorf("could not reach %s: %w", addr, err)
			}
			if info.Persist {
				m, err := client.ConnectAddress(gamesDir, addr)
				if err != nil {
					return err
				}
				_, err = tea.NewProgram(m).Run()
				return err
			}
		}
		password := ""
		if account != "" {
			pw, err := promptSecret("Password: ")
			if err != nil {
				return err
			}
			if pw == "" {
				return fmt.Errorf("password cannot be empty")
			}
			password = pw
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sess, cleanup, err := snet.DialWithLogin(ctx, addr, snet.LoginOptions{
			Account:  account,
			Password: password,
			Name:     name,
			Race:     race,
			Role:     role,
		})
		if err != nil {
			return err
		}
		defer cleanup()
		m := client.ConnectPlay(sess, cleanup)
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
		fmt.Print(`Erickson Stories / sudengine — a MUD engine

Usage:
  sudengine                 Launcher (pick a pack)
  sudengine play <pack>     Play a pack
  sudengine build <pack>    Build/edit a pack in-engine
  sudengine serve <pack>    Serve a pack over framed protocol
  sudengine connect <addr>  Connect to a remote server
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
