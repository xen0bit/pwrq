package net

import (
	"fmt"
	"testing"
)

func TestIPVersion(t *testing.T) {
	if got := fmt.Sprint(run(t, `"192.168.1.1" | ip_version`)); got != "v4" {
		t.Errorf("ip_version v4 = %s", got)
	}
	if got := fmt.Sprint(run(t, `"2001:db8::1" | ip_version`)); got != "v6" {
		t.Errorf("ip_version v6 = %s", got)
	}
}

func TestPrivateLoopback(t *testing.T) {
	if got := run(t, `"10.1.2.3" | is_private_ip`); got != true {
		t.Error("10.1.2.3 should be private")
	}
	if got := run(t, `"8.8.8.8" | is_private_ip`); got != false {
		t.Error("8.8.8.8 should be public")
	}
	if got := run(t, `"127.0.0.1" | is_loopback`); got != true {
		t.Error("127.0.0.1 should be loopback")
	}
	if got := run(t, `"::1" | is_loopback`); got != true {
		t.Error("::1 should be loopback")
	}
}

func TestCidrBounds(t *testing.T) {
	if got := fmt.Sprint(run(t, `"192.168.1.55/24" | cidr_network`)); got != "192.168.1.0" {
		t.Errorf("cidr_network = %s", got)
	}
	if got := fmt.Sprint(run(t, `"192.168.1.55/24" | cidr_broadcast`)); got != "192.168.1.255" {
		t.Errorf("cidr_broadcast = %s", got)
	}
}

func TestIPAdd(t *testing.T) {
	if got := fmt.Sprint(run(t, `"192.168.1.1" | ip_add(1)`)); got != "192.168.1.2" {
		t.Errorf("ip_add(1) = %s", got)
	}
	if got := fmt.Sprint(run(t, `"192.168.1.1" | ip_add(-1)`)); got != "192.168.1.0" {
		t.Errorf("ip_add(-1) = %s", got)
	}
}

func TestIPv6Expand(t *testing.T) {
	if got := fmt.Sprint(run(t, `"2001:db8::1" | ipv6_expand`)); got != "2001:0db8:0000:0000:0000:0000:0000:0001" {
		t.Errorf("ipv6_expand = %s", got)
	}
}

func TestReverseIP(t *testing.T) {
	if got := fmt.Sprint(run(t, `"192.168.1.1" | reverse_ip`)); got != "1.1.168.192.in-addr.arpa." {
		t.Errorf("reverse_ip v4 = %s", got)
	}
	if got := fmt.Sprint(run(t, `"2001:db8::1" | reverse_ip`)); got != "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa." {
		t.Errorf("reverse_ip v6 = %s", got)
	}
}
