package main

import (
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestServeRejectsBuild(t *testing.T) {
	err := run([]string{"serve", "old-house", "build"})
	if err == nil {
		t.Fatalf("expected error for serve in build mode, got nil")
	}
	if !strings.Contains(err.Error(), "build mode is not permitted") {
		t.Fatalf("unexpected error message: %v", err)
	}

	err = run([]string{"serve", "old-house", "--build"})
	if err == nil {
		t.Fatalf("expected error for serve with --build flag, got nil")
	}
}

func TestUsageErrors(t *testing.T) {
	err := run([]string{"serve"})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error for serve without pack, got %v", err)
	}

	err = run([]string{"connect"})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error for connect without host:port, got %v", err)
	}
}

func TestServeRejectsBadPort(t *testing.T) {
	for _, args := range [][]string{
		{"serve", "old-house", "--port"},
		{"serve", "old-house", "--port", "0"},
		{"serve", "old-house", "--port", "70000"},
		{"serve", "old-house", "--port", "nope"},
	} {
		err := run(args)
		if err == nil {
			t.Fatalf("expected port error for %q", args)
		}
	}
}

func TestServeBindErrorExitsNonZero(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	dir := filepath.Join("..", "..", "games", "old-house")
	err = run([]string{"serve", dir, "--port", strconv.Itoa(port)})
	if err == nil {
		t.Fatal("expected bind failure to return an error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "bind") && !strings.Contains(strings.ToLower(err.Error()), "listen") {
		t.Fatalf("expected a listen error, got %v", err)
	}
}
