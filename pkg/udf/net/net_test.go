package net

import (
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(nil)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestIsIP(t *testing.T) {
	if got := run(t, `"192.168.1.1" | is_ip`); got != true {
		t.Error("is_ip(192.168.1.1) = false")
	}
	if got := run(t, `"2001:db8::1" | is_ip`); got != true {
		t.Error("is_ip(2001:db8::1) = false")
	}
	if got := run(t, `"not an ip" | is_ip`); got != false {
		t.Error("is_ip(garbage) = true")
	}
}

func TestIsIPv4IPv6(t *testing.T) {
	if got := run(t, `"192.168.1.1" | is_ipv4`); got != true {
		t.Error("is_ipv4 = false")
	}
	if got := run(t, `"2001:db8::1" | is_ipv4`); got != false {
		t.Error("is_ipv4(v6) = true")
	}
	if got := run(t, `"2001:db8::1" | is_ipv6`); got != true {
		t.Error("is_ipv6 = false")
	}
	if got := run(t, `"192.168.1.1" | is_ipv6`); got != false {
		t.Error("is_ipv6(v4) = true")
	}
}

func TestIPToInt(t *testing.T) {
	if got := fmt.Sprint(run(t, `"192.168.1.1" | ip_to_int`)); got != "3232235777" {
		t.Errorf("ip_to_int = %s", got)
	}
	if got := fmt.Sprint(run(t, `"0.0.0.0" | ip_to_int`)); got != "0" {
		t.Errorf("ip_to_int zero = %s", got)
	}
}

func TestIntToIP(t *testing.T) {
	if got := fmt.Sprint(run(t, `3232235777 | int_to_ip`)); got != "192.168.1.1" {
		t.Errorf("int_to_ip = %s", got)
	}
	if got := fmt.Sprint(run(t, `0 | int_to_ip`)); got != "0.0.0.0" {
		t.Errorf("int_to_ip zero = %s", got)
	}
}

func TestInCidr(t *testing.T) {
	if got := run(t, `"10.1.2.3" | in_cidr("10.0.0.0/8")`); got != true {
		t.Error("10.1.2.3 not in 10/8")
	}
	if got := run(t, `"192.168.1.1" | in_cidr("10.0.0.0/8")`); got != false {
		t.Error("192.168.1.1 in 10/8")
	}
}

func TestCidrSize(t *testing.T) {
	if got := fmt.Sprint(run(t, `"10.0.0.0/24" | cidr_size`)); got != "256" {
		t.Errorf("cidr_size /24 = %s", got)
	}
	if got := fmt.Sprint(run(t, `"10.0.0.0/32" | cidr_size`)); got != "1" {
		t.Errorf("cidr_size /32 = %s", got)
	}
}

func TestMac(t *testing.T) {
	if got := run(t, `"00:11:22:33:44:55" | is_mac`); got != true {
		t.Error("is_mac colon = false")
	}
	if got := run(t, `"00-11-22-33-44-55" | is_mac`); got != true {
		t.Error("is_mac hyphen = false")
	}
	if got := run(t, `"notamac" | is_mac`); got != false {
		t.Error("is_mac garbage = true")
	}
	if got := fmt.Sprint(run(t, `"00-11-22-33-44-55" | mac_normalize`)); got != "00:11:22:33:44:55" {
		t.Errorf("mac_normalize hyphen = %s", got)
	}
	if got := fmt.Sprint(run(t, `"0011.2233.4455" | mac_normalize`)); got != "00:11:22:33:44:55" {
		t.Errorf("mac_normalize cisco = %s", got)
	}
}
