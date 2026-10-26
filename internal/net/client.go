package net

import (
	"context"
	stdnet "net"
	"sync"

	"sudengine/internal/protocol"
	"sudengine/internal/server"
)

// Dial connects to a remote sudengine server over TCP using the framed protocol,
// performs the connect handshake, and returns a Session connected to the socket.
func Dial(ctx context.Context, addr string, name, race, role string) (*server.Session, context.CancelFunc, error) {
	var d stdnet.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	connectPayload, err := EncodeConnect(name, race, role)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if err := WriteFrame(conn, connectPayload); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	sess := &server.Session{
		ID:  name,
		In:  make(chan string, 32),
		Out: make(chan protocol.Event, 128),
	}

	ctx, cancel := context.WithCancel(ctx)
	connClosed := make(chan struct{})
	var closeOnce sync.Once
	closeConn := func() {
		closeOnce.Do(func() { close(connClosed) })
	}

	go func() {
		defer func() {
			_ = conn.Close()
			cancel()
		}()
		for {
			select {
			case <-connClosed:
				return
			case <-ctx.Done():
				return
			case line, ok := <-sess.In:
				if !ok {
					return
				}
				data, err := EncodeCommand(line)
				if err != nil {
					continue
				}
				if err := WriteFrame(conn, data); err != nil {
					return
				}
			}
		}
	}()

	go func() {
		defer func() {
			closeConn()
			_ = conn.Close()
			sess.Disconnect("connection closed")
			cancel()
		}()
		for {
			payload, err := ReadFrame(conn)
			if err != nil {
				return
			}
			ev, err := DecodeEvent(payload)
			if err != nil {
				continue
			}
			sess.Send(ev)
			if _, ok := ev.(protocol.DisconnectEvent); ok {
				return
			}
		}
	}()

	cleanup := func() {
		cancel()
		closeConn()
		_ = conn.Close()
	}

	return sess, cleanup, nil
}
