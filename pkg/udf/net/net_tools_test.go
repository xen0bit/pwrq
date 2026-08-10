package net

import (
	"fmt"
	"testing"
)

func TestSubnetOf(t *testing.T) {
	if got := run(t, `"10.0.0.0/24" | subnet_of("10.0.0.0/8")`); got != true {
		t.Error("10/24 not subnet of 10/8")
	}
	if got := run(t, `"10.0.0.0/8" | subnet_of("10.0.0.0/24")`); got != false {
		t.Error("10/8 subnet of 10/24")
	}
}

func TestCidrHosts(t *testing.T) {
	if got := fmt.Sprint(run(t, `"10.0.0.0/24" | cidr_first_host`)); got != "10.0.0.1" {
		t.Errorf("cidr_first_host = %s", got)
	}
	if got := fmt.Sprint(run(t, `"10.0.0.0/24" | cidr_last_host`)); got != "10.0.0.254" {
		t.Errorf("cidr_last_host = %s", got)
	}
}

func TestIsPublicIP(t *testing.T) {
	if got := run(t, `"8.8.8.8" | is_public_ip`); got != true {
		t.Error("8.8.8.8 should be public")
	}
	if got := run(t, `"10.0.0.1" | is_public_ip`); got != false {
		t.Error("10.0.0.1 should not be public")
	}
	if got := run(t, `"127.0.0.1" | is_public_ip`); got != false {
		t.Error("127.0.0.1 should not be public")
	}
}

func TestPortName(t *testing.T) {
	if got := fmt.Sprint(run(t, `443 | port_name`)); got != "https" {
		t.Errorf("port_name(443) = %s", got)
	}
	if got := fmt.Sprint(run(t, `5432 | port_name`)); got != "postgresql" {
		t.Errorf("port_name(5432) = %s", got)
	}
	if got := fmt.Sprint(run(t, `12345 | port_name`)); got != "unknown" {
		t.Errorf("port_name(12345) = %s", got)
	}
}
