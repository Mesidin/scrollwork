package net

import (
	"context"
	"errors"
	"fmt"
	stdnet "net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"sudengine/internal/protocol"
	"sudengine/internal/server"
)

// NormalizeAddr validates and normalizes the server listen address.
// The server only binds to 127.0.0.1 and refuses public or wildcard addresses.
func NormalizeAddr(addr string) (string, error) {
	if addr == "" {
		return "127.0.0.1:4000", nil
	}
	if !strings.Contains(addr, ":") {
		port, err := strconv.Atoi(addr)
		if err != nil || port <= 0 || port > 65535 {
			return "", fmt.Errorf("invalid port %q", addr)
		}
		return fmt.Sprintf("127.0.0.1:%d", port), nil
	}
	host, portStr, err := stdnet.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if host == "" || host == "localhost" || host == "127.0.0.1" {
		host = "127.0.0.1"
	} else {
		return "", fmt.Errorf("serve only listens on 127.0.0.1, got host %q", host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return "", fmt.Errorf("invalid port %q", portStr)
	}
	return stdnet.JoinHostPort(host, portStr), nil
}

// Listen creates a TCP listener bound to 127.0.0.1 on the specified address/port.
func Listen(addr string) (stdnet.Listener, error) {
	norm, err := NormalizeAddr(addr)
	if err != nil {
		return nil, err
	}
	return stdnet.Listen("tcp", norm)
}

// ListenAndServe listens on addr and serves framed connections for srv until ctx is canceled.
func ListenAndServe(ctx context.Context, srv *server.Server, addr string) error {
	ln, err := Listen(addr)
	if err != nil {
		return err
	}
	return Serve(ctx, srv, ln)
}

// Serve serves framed connections on ln for srv until ctx is canceled or ln is closed.
// One accept error does not stop the listener.
func Serve(ctx context.Context, srv *server.Server, ln stdnet.Listener) error {
	defer ln.Close()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, stdnet.ErrClosed) {
				return nil
			}
			fmt.Fprintf(os.Stderr, "accept: %v\n", err)
			timer := time.NewTimer(20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}
		go ServeConn(ctx, srv, conn)
	}
}

const (
	handshakeTimeout = 10 * time.Second
	frameBodyTimeout = 5 * time.Second
)

// ServeConn handles a single framed TCP client connection.
// The connection goroutine only enqueues. The actor spawns the player.
func ServeConn(ctx context.Context, srv *server.Server, conn stdnet.Conn) {
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	payload, err := ReadFrame(conn)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return
	}
	cf, err := DecodeClientFrame(payload)
	if err != nil {
		return
	}
	if cf.Type != "connect" && cf.Type != "command" {
		return
	}

	sess := &server.Session{
		Out: make(chan protocol.Event, 128),
		In:  make(chan string, 32),
	}
	first := ""
	if cf.Type == "command" {
		first = cf.Command
	}
	if !srv.EnqueueConnect(sess, cf.Name, cf.Race, cf.Role, first) {
		return
	}

	writerDone := make(chan struct{})
	connClosed := make(chan struct{})
	var closeOnce sync.Once
	signalClose := func() {
		closeOnce.Do(func() { close(connClosed) })
	}

	// A past read deadline does not unblock a write stuck on a full window.
	// Closing the socket does, and only on cancel — a stall still needs the
	// writer to flush the disconnect once the peer reads.
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-connClosed:
		}
	}()

	go writeLoop(ctx, conn, sess, connClosed, writerDone)

	reason := "closed"
	for {
		if sess.Closed() {
			break
		}
		payload, err := readFrameBodyDeadline(conn, frameBodyTimeout)
		if err != nil {
			break
		}
		frame, err := DecodeClientFrame(payload)
		if err != nil {
			continue
		}
		switch frame.Type {
		case "command":
			if !srv.Enqueue(sess, frame.Command) {
				goto done
			}
		case "disconnect":
			if frame.Reason != "" {
				reason = frame.Reason
			}
			goto done
		}
	}

done:
	_ = srv.EnqueueDisconnect(sess, reason)
	waitSessionClosed(srv, sess)
	signalClose()
	<-writerDone
}

func waitSessionClosed(srv *server.Server, sess *server.Session) {
	if sess.Closed() {
		return
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	stopped := srv.Stopped()
	for !sess.Closed() {
		if stopped == nil {
			select {
			case <-timer.C:
				return
			case <-tick.C:
			}
			continue
		}
		select {
		case <-stopped:
			return
		case <-timer.C:
			return
		case <-tick.C:
		}
	}
}

func writeLoop(ctx context.Context, conn stdnet.Conn, sess *server.Session, connClosed <-chan struct{}, writerDone chan struct{}) {
	defer close(writerDone)
	defer conn.Close()
	// Flush is not visible while this goroutine is blocked on Out. A full
	// buffer leaves DisconnectEvent in the side queue until the next wake.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		sess.Flush()
		select {
		case <-connClosed:
			drainSession(conn, sess)
			return
		case ev, ok := <-sess.Out:
			if !ok {
				return
			}
			if err := writeOne(conn, ev); err != nil {
				return
			}
			if _, isDisc := ev.(protocol.DisconnectEvent); isDisc {
				return
			}
		case <-ticker.C:
			if ctx.Err() != nil && sess.Closed() && !sess.HasOutbound() {
				return
			}
		}
	}
}

func drainSession(conn stdnet.Conn, sess *server.Session) {
	sess.Flush()
	for {
		select {
		case ev, ok := <-sess.Out:
			if !ok {
				return
			}
			if err := writeOne(conn, ev); err != nil {
				return
			}
			sess.Flush()
			if _, isDisc := ev.(protocol.DisconnectEvent); isDisc {
				return
			}
		default:
			return
		}
	}
}

func writeOne(conn stdnet.Conn, ev protocol.Event) error {
	data, err := EncodeEvent(ev)
	if err != nil {
		return nil
	}
	return WriteFrame(conn, data)
}
