package net

import (
	"context"
	"encoding/json"
	"fmt"
	stdnet "net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sudengine/internal/protocol"
	"sudengine/internal/server"
)

// LoginOptions holds credentials and character choices for a framed connection.
type LoginOptions struct {
	Account  string
	Password string
	Name     string
	Race     string
	Role     string
	Attrs    map[string]int
}

// NormalizeClientAddr normalizes a client target address, defaulting to 127.0.0.1:4000
// when empty and appending port 4000 when no port is specified.
func NormalizeClientAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "127.0.0.1:4000"
	}
	if _, err := strconv.Atoi(addr); err == nil {
		return "127.0.0.1:" + addr
	}
	if !strings.Contains(addr, ":") {
		return addr + ":4000"
	}
	host, port, err := stdnet.SplitHostPort(addr)
	if err == nil {
		if host == "" {
			host = "127.0.0.1"
		}
		if port == "" {
			port = "4000"
		}
		return stdnet.JoinHostPort(host, port)
	}
	return addr
}

// Probe connects briefly to addr, sends a "ping" frame, and returns the server's metadata.
func Probe(ctx context.Context, addr string) (ServerInfo, error) {
	addr = NormalizeClientAddr(addr)
	d := stdnet.Dialer{Timeout: 800 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return ServerInfo{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))

	ping, err := EncodePing()
	if err != nil {
		return ServerInfo{}, err
	}
	if err := WriteFrame(conn, ping); err != nil {
		return ServerInfo{}, err
	}
	payload, err := ReadFrame(conn)
	if err != nil {
		return ServerInfo{}, err
	}
	var info ServerInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return ServerInfo{}, err
	}
	if info.Type != "server_info" {
		return ServerInfo{}, fmt.Errorf("unexpected response from %s", addr)
	}
	info.Addr = addr
	return info, nil
}

// Authenticate verifies (or creates) an account on the remote server and returns
// the server metadata along with the account's existing characters.
func Authenticate(ctx context.Context, addr, account, password string) (ServerInfo, error) {
	addr = NormalizeClientAddr(addr)
	d := stdnet.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return ServerInfo{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))

	frame, err := EncodeAuth(account, password)
	if err != nil {
		return ServerInfo{}, err
	}
	if err := WriteFrame(conn, frame); err != nil {
		return ServerInfo{}, err
	}
	payload, err := ReadFrame(conn)
	if err != nil {
		return ServerInfo{}, err
	}
	var info ServerInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return ServerInfo{}, err
	}
	info.Addr = addr
	if !info.OK {
		if info.Error != "" {
			return info, fmt.Errorf("%s", info.Error)
		}
		return info, fmt.Errorf("authentication failed")
	}
	return info, nil
}

// LocalLANIPs returns active private IPv4 addresses on this machine.
func LocalLANIPs() []string {
	ifaces, err := stdnet.Interfaces()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var ips []string
	for _, iface := range ifaces {
		if skipLANInterface(iface) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip stdnet.IP
			switch v := a.(type) {
			case *stdnet.IPNet:
				ip = v.IP
			case *stdnet.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil || !ip4.IsPrivate() {
				continue
			}
			s := ip4.String()
			if !seen[s] {
				seen[s] = true
				ips = append(ips, s)
			}
		}
	}
	sort.Strings(ips)
	return ips
}

// skipLANInterface leaves out interfaces that are down, loopback, or a known
// virtual switch. Those adapters are private on Windows (WSL, Hyper-V, Docker)
// and a failed bind on one of them used to disappear.
func skipLANInterface(iface stdnet.Interface) bool {
	if iface.Flags&stdnet.FlagUp == 0 || iface.Flags&stdnet.FlagLoopback != 0 {
		return true
	}
	return virtualIface(iface.Name)
}

func virtualIface(name string) bool {
	n := strings.ToLower(name)
	for _, p := range []string{
		"vethernet", "vmware", "virtualbox", "vbox", "docker", "wsl", "hyper-v", "hyperv",
		"bluetooth", "teredo", "isatap", "loopback", "tunnel", "npcap", "virbr", "vboxnet",
	} {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}

// DiscoverServers scans 127.0.0.1 and the local /24 private LAN subnets on port
// (default 4000) for running sudengine servers.
func DiscoverServers(ctx context.Context, port int) []ServerInfo {
	if port <= 0 {
		port = 4000
	}
	portStr := strconv.Itoa(port)
	selfIPs := LocalLANIPs()
	selfSet := map[string]bool{"127.0.0.1": true}
	for _, ip := range selfIPs {
		selfSet[ip] = true
	}

	var candidates []string
	candidates = append(candidates, stdnet.JoinHostPort("127.0.0.1", portStr))

	seenSubnets := map[string]bool{}
	for _, ipStr := range selfIPs {
		ip := stdnet.ParseIP(ipStr).To4()
		if ip == nil {
			continue
		}
		prefix := fmt.Sprintf("%d.%d.%d.", ip[0], ip[1], ip[2])
		if seenSubnets[prefix] {
			continue
		}
		seenSubnets[prefix] = true
		for host := 1; host <= 254; host++ {
			candIP := prefix + strconv.Itoa(host)
			if selfSet[candIP] {
				continue
			}
			candidates = append(candidates, stdnet.JoinHostPort(candIP, portStr))
		}
	}

	var (
		mu      sync.Mutex
		found   []ServerInfo
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 64)
		timeout = 180 * time.Millisecond
	)

	probeOne := func(target string) {
		defer wg.Done()
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		case <-ctx.Done():
			return
		}
		pCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		d := stdnet.Dialer{Timeout: timeout}
		conn, err := d.DialContext(pCtx, "tcp", target)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
		ping, err := EncodePing()
		if err != nil {
			return
		}
		if err := WriteFrame(conn, ping); err != nil {
			return
		}
		payload, err := ReadFrame(conn)
		if err != nil {
			return
		}
		var info ServerInfo
		if err := json.Unmarshal(payload, &info); err != nil || info.Type != "server_info" {
			return
		}
		info.Addr = target
		mu.Lock()
		found = append(found, info)
		mu.Unlock()
	}

	for _, c := range candidates {
		wg.Add(1)
		go probeOne(c)
	}
	wg.Wait()

	sort.Slice(found, func(i, j int) bool {
		if strings.HasPrefix(found[i].Addr, "127.0.0.1:") != strings.HasPrefix(found[j].Addr, "127.0.0.1:") {
			return strings.HasPrefix(found[i].Addr, "127.0.0.1:")
		}
		return found[i].Addr < found[j].Addr
	})
	return found
}

// Dial connects to a remote sudengine server over TCP using the framed protocol,
// performs the connect handshake, and returns a Session connected to the socket.
func Dial(ctx context.Context, addr string, name, race, role string) (*server.Session, context.CancelFunc, error) {
	return DialWithLogin(ctx, addr, LoginOptions{
		Name: name,
		Race: race,
		Role: role,
	})
}

// DialWithLogin connects to a remote sudengine server with optional account credentials
// and character creation attributes.
func DialWithLogin(ctx context.Context, addr string, opt LoginOptions) (*server.Session, context.CancelFunc, error) {
	addr = NormalizeClientAddr(addr)
	if ctx == nil {
		ctx = context.Background()
	}
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()
	d := stdnet.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	connectPayload, err := EncodeConnectAuth(opt.Account, opt.Password, opt.Name, opt.Race, opt.Role, opt.Attrs)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if err := WriteFrame(conn, connectPayload); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	sessID := opt.Name
	if sessID == "" {
		sessID = "you"
	}
	sess := &server.Session{
		ID:      sessID,
		Account: opt.Account,
		In:      make(chan string, 32),
		Out:     make(chan protocol.Event, 128),
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
