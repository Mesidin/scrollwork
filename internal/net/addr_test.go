package net

import "testing"

func TestNormalizeAddrRejectsUnspecified(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:4000", "[::]:4000", "0.0.0.0:0"} {
		if _, err := NormalizeAddr(addr); err == nil {
			t.Fatalf("expected %s to be rejected", addr)
		}
	}
	got, err := NormalizeAddr("127.0.0.1:4000")
	if err != nil || got != "127.0.0.1:4000" {
		t.Fatalf("loopback: %q %v", got, err)
	}
	got, err = NormalizeAddr("")
	if err != nil || got != "127.0.0.1:4000" {
		t.Fatalf("default: %q %v", got, err)
	}
}

func TestVirtualIfaceNames(t *testing.T) {
	for _, name := range []string{"vEthernet (WSL)", "VMware Network Adapter", "DockerNAT", "Hyper-V Virtual Ethernet"} {
		if !virtualIface(name) {
			t.Fatalf("expected %q to be treated as virtual", name)
		}
	}
	if virtualIface("Ethernet") || virtualIface("Wi-Fi") {
		t.Fatal("expected physical adapter names to stay")
	}
}
