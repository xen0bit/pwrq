package net

import (
	"fmt"
	"net/netip"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSubnetOf registers subnet_of, whether one CIDR block is inside
// another.
func RegisterSubnetOf() gojq.CompilerOption {
	return gojq.WithFunction("subnet_of", 1, 1, func(v any, args []any) any {
		subnet, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("subnet_of: expected a CIDR string, got %T", v), nil)
		}
		super, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("subnet_of: expected a CIDR string, got %T", args[0]), nil)
		}
		sub, err := netip.ParsePrefix(subnet)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("subnet_of: %q is not a CIDR", subnet), nil)
		}
		sup, err := netip.ParsePrefix(super)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("subnet_of: %q is not a CIDR", super), nil)
		}
		return common.MakeUDFSuccessResult(sub.Bits() >= sup.Bits() && sup.Contains(sub.Masked().Addr()), nil)
	})
}

// RegisterCidrFirstHost registers cidr_first_host, the first usable host
// address of a CIDR block.
func RegisterCidrFirstHost() gojq.CompilerOption {
	return gojq.WithFunction("cidr_first_host", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "cidr_first_host")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_first_host: %v", err), nil)
		}
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_first_host: %q is not a CIDR", s), nil)
		}
		return common.MakeUDFSuccessResult(addToAddr(prefix.Masked().Addr(), 1).String(), nil)
	})
}

// RegisterCidrLastHost registers cidr_last_host, the last usable host address
// of a CIDR block.
func RegisterCidrLastHost() gojq.CompilerOption {
	return gojq.WithFunction("cidr_last_host", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "cidr_last_host")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_last_host: %v", err), nil)
		}
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_last_host: %q is not a CIDR", s), nil)
		}
		return common.MakeUDFSuccessResult(addToAddr(lastAddress(prefix), -1).String(), nil)
	})
}

// RegisterIsPublicIP registers is_public_ip, whether an address is not private,
// loopback, link-local, multicast or unspecified.
func RegisterIsPublicIP() gojq.CompilerOption {
	return registerBool("is_public_ip", func(s string) bool {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return false
		}
		return !(addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
			addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified())
	})
}

var commonPorts = map[int64]string{
	20: "ftp-data", 21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp",
	53: "dns", 67: "dhcp", 68: "dhcp", 80: "http", 110: "pop3",
	123: "ntp", 143: "imap", 443: "https", 445: "smb", 465: "smtps",
	514: "syslog", 587: "smtp", 993: "imaps", 995: "pop3s",
	1433: "mssql", 1521: "oracle", 3306: "mysql", 3389: "rdp",
	5432: "postgresql", 5900: "vnc", 6379: "redis", 8080: "http-alt",
	8443: "https-alt", 9092: "kafka", 9200: "elasticsearch",
	11211: "memcached", 27017: "mongodb",
}

// RegisterPortName registers port_name, the common service name for a port
// number.
func RegisterPortName() gojq.CompilerOption {
	return gojq.WithFunction("port_name", 0, 0, func(v any, args []any) any {
		port, ok := common.ToInt(common.BindValue(v))
		if !ok || port < 0 || port > 65535 {
			return common.MakeUDFErrorResult(fmt.Errorf("port_name: expected a port 0-65535, got %v", v), nil)
		}
		if name, known := commonPorts[int64(port)]; known {
			return common.MakeUDFSuccessResult(name, nil)
		}
		return common.MakeUDFSuccessResult("unknown", nil)
	})
}
