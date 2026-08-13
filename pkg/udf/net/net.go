// Package net provides IP and MAC address utilities: validation, integer
// conversion, CIDR containment and sizing, and MAC normalization. It is pure,
// built on net/netip, so it runs in the browser too.
package net

import (
	"fmt"
	"math"
	"math/big"
	"net/netip"
	"regexp"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every network cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterIsIP(),
		RegisterIsIPv4(),
		RegisterIsIPv6(),
		RegisterIPToInt(),
		RegisterIntToIP(),
		RegisterInCidr(),
		RegisterCidrSize(),
		RegisterIsMac(),
		RegisterMacNormalize(),
		RegisterIPVersion(),
		RegisterIsPrivateIP(),
		RegisterIsLoopback(),
		RegisterCidrNetwork(),
		RegisterCidrBroadcast(),
		RegisterIPAdd(),
		RegisterIPv6Expand(),
		RegisterReverseIP(),
		RegisterSubnetOf(),
		RegisterCidrFirstHost(),
		RegisterCidrLastHost(),
		RegisterIsPublicIP(),
		RegisterPortName(),
	}
}

// addToAddr shifts an address by n positions, wrapping within its family.
func addToAddr(addr netip.Addr, n int64) netip.Addr {
	bitLen := addr.BitLen()
	value := newBigFromAddr(addr)
	value.Add(value, big.NewInt(n))
	mod := new(big.Int).Lsh(big.NewInt(1), uint(bitLen))
	value.Mod(value, mod)
	bytes := value.Bytes()
	if bitLen == 32 {
		var b [4]byte
		copy(b[4-len(bytes):], bytes)
		return netip.AddrFrom4(b)
	}
	var b [16]byte
	copy(b[16-len(bytes):], bytes)
	return netip.AddrFrom16(b)
}

func newBigFromAddr(addr netip.Addr) *big.Int {
	if addr.Is4() {
		bytes := addr.As4()
		return new(big.Int).SetUint64(uint64(bytes[0])<<24 | uint64(bytes[1])<<16 | uint64(bytes[2])<<8 | uint64(bytes[3]))
	}
	bytes := addr.As16()
	return new(big.Int).SetBytes(bytes[:])
}

// strInput resolves a string from the pipeline or first argument.
func strInput(v any, args []any, name string) (string, error) {
	inputVal, _, err := common.ParseFileArgs(v, args)
	if err != nil {
		return "", err
	}
	switch val := common.BindValue(inputVal).(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return "", fmt.Errorf("%s: expected a string, got %T", name, inputVal)
	}
}

// registerBool registers a 0-2 arity predicate over a string.
func registerBool(name string, fn func(string) bool) gojq.CompilerOption {
	return common.WithFunction(name, 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, name)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return common.MakeUDFSuccessResult(fn(s), nil)
	})
}

// RegisterIsIP registers is_ip, whether a string is a valid IPv4 or IPv6
// address.
func RegisterIsIP() gojq.CompilerOption {
	return registerBool("is_ip", func(s string) bool {
		_, err := netip.ParseAddr(s)
		return err == nil
	})
}

// RegisterIsIPv4 registers is_ipv4, whether a string is a plain IPv4 address.
func RegisterIsIPv4() gojq.CompilerOption {
	return registerBool("is_ipv4", func(s string) bool {
		addr, err := netip.ParseAddr(s)
		return err == nil && addr.Is4()
	})
}

// RegisterIsIPv6 registers is_ipv6, whether a string is an IPv6 address.
func RegisterIsIPv6() gojq.CompilerOption {
	return registerBool("is_ipv6", func(s string) bool {
		addr, err := netip.ParseAddr(s)
		return err == nil && addr.Is6()
	})
}

// RegisterIPToInt registers ip_to_int, an address as a decimal string (a plain
// number for IPv4, a 128-bit decimal for IPv6 — returned as text so no
// precision is lost).
func RegisterIPToInt() gojq.CompilerOption {
	return common.WithFunction("ip_to_int", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "ip_to_int")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ip_to_int: %v", err), nil)
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ip_to_int: %q is not an address: %v", s, err), nil)
		}
		if addr.Is4() {
			bytes := addr.As4()
			return common.MakeUDFSuccessResult(uint32(bytes[0])<<24|uint32(bytes[1])<<16|uint32(bytes[2])<<8|uint32(bytes[3]), nil)
		}
		bytes := addr.As16()
		n := new(big.Int).SetBytes(bytes[:])
		return common.MakeUDFSuccessResult(n.String(), nil)
	})
}

// RegisterIntToIP registers int_to_ip, a decimal integer back to an address.
// Values up to 2^32-1 become IPv4; larger values become IPv6.
func RegisterIntToIP() gojq.CompilerOption {
	return common.WithFunction("int_to_ip", 0, 1, func(v any, args []any) any {
		var f float64
		var ok bool
		if len(args) > 0 {
			f, ok = common.ToFloat64(common.BindValue(args[0]))
		} else {
			f, ok = common.ToFloat64(common.BindValue(v))
		}
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("int_to_ip: expected a number, got %T", v), nil)
		}
		if f < 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("int_to_ip: cannot represent a negative integer"), nil)
		}
		n := new(big.Int).SetUint64(uint64(f))
		if n.BitLen() <= 32 {
			var bytes [4]byte
			raw := n.Bytes()
			copy(bytes[4-len(raw):], raw)
			return common.MakeUDFSuccessResult(netip.AddrFrom4(bytes).String(), nil)
		}
		var bytes [16]byte
		raw := n.Bytes()
		copy(bytes[16-len(raw):], raw)
		return common.MakeUDFSuccessResult(netip.AddrFrom16(bytes).String(), nil)
	})
}

// RegisterInCidr registers in_cidr, whether an address falls inside a CIDR
// block.
func RegisterInCidr() gojq.CompilerOption {
	return common.WithFunction("in_cidr", 1, 1, func(v any, args []any) any {
		cidr, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("in_cidr: expected a CIDR string, got %T", args[0]), nil)
		}
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("in_cidr: %q is not a CIDR: %v", cidr, err), nil)
		}
		ip, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("in_cidr: expected an IP string, got %T", v), nil)
		}
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("in_cidr: %q is not an address: %v", ip, err), nil)
		}
		return common.MakeUDFSuccessResult(prefix.Contains(addr), nil)
	})
}

// RegisterCidrSize registers cidr_size, how many addresses a CIDR block holds.
func RegisterCidrSize() gojq.CompilerOption {
	return common.WithFunction("cidr_size", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "cidr_size")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_size: %v", err), nil)
		}
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_size: %q is not a CIDR: %v", s, err), nil)
		}
		bitLen := prefix.Addr().BitLen()
		hostBits := bitLen - prefix.Bits()
		if hostBits <= 63 {
			return common.MakeUDFSuccessResult(int64(1)<<uint(hostBits), nil)
		}
		return common.MakeUDFSuccessResult(math.Pow(2, float64(hostBits)), nil)
	})
}

var macPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^[0-9A-Fa-f]{2}([:-][0-9A-Fa-f]{2}){5}$`),
	regexp.MustCompile(`^[0-9A-Fa-f]{4}(\.[0-9A-Fa-f]{4}){2}$`),
}

// RegisterIsMac registers is_mac, whether a string is a MAC address in any
// common separator style.
func RegisterIsMac() gojq.CompilerOption {
	return registerBool("is_mac", func(s string) bool {
		for _, re := range macPatterns {
			if re.MatchString(s) {
				return true
			}
		}
		return false
	})
}

// RegisterMacNormalize registers mac_normalize, lowercasing a MAC address and
// rendering it colon-separated.
func RegisterMacNormalize() gojq.CompilerOption {
	return common.WithFunction("mac_normalize", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "mac_normalize")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("mac_normalize: %v", err), nil)
		}
		clean := strings.ToLower(strings.Map(func(r rune) rune {
			switch r {
			case ':', '-', '.':
				return -1
			}
			return r
		}, strings.TrimSpace(s)))
		if len(clean) != 12 {
			return common.MakeUDFErrorResult(fmt.Errorf("mac_normalize: %q is not a 12-digit MAC address", s), nil)
		}
		parts := make([]string, 6)
		for i := range parts {
			parts[i] = clean[i*2 : i*2+2]
		}
		return common.MakeUDFSuccessResult(strings.Join(parts, ":"), nil)
	})
}

// RegisterIPVersion registers ip_version, "v4" or "v6" for an address.
func RegisterIPVersion() gojq.CompilerOption {
	return common.WithFunction("ip_version", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "ip_version")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ip_version: %v", err), nil)
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ip_version: %q is not an address", s), nil)
		}
		if addr.Is4() {
			return common.MakeUDFSuccessResult("v4", nil)
		}
		return common.MakeUDFSuccessResult("v6", nil)
	})
}

// RegisterIsPrivateIP registers is_private_ip, whether an address is private or
// loopback (RFC 1918, ULA, 127/8, ::1).
func RegisterIsPrivateIP() gojq.CompilerOption {
	return registerBool("is_private_ip", func(s string) bool {
		addr, err := netip.ParseAddr(s)
		return err == nil && (addr.IsPrivate() || addr.IsLoopback())
	})
}

// RegisterIsLoopback registers is_loopback, whether an address is loopback.
func RegisterIsLoopback() gojq.CompilerOption {
	return registerBool("is_loopback", func(s string) bool {
		addr, err := netip.ParseAddr(s)
		return err == nil && addr.IsLoopback()
	})
}

// RegisterCidrNetwork registers cidr_network, the base address of a CIDR
// block.
func RegisterCidrNetwork() gojq.CompilerOption {
	return common.WithFunction("cidr_network", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "cidr_network")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_network: %v", err), nil)
		}
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_network: %q is not a CIDR", s), nil)
		}
		return common.MakeUDFSuccessResult(prefix.Masked().Addr().String(), nil)
	})
}

// RegisterCidrBroadcast registers cidr_broadcast, the last address of a CIDR
// block.
func RegisterCidrBroadcast() gojq.CompilerOption {
	return common.WithFunction("cidr_broadcast", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "cidr_broadcast")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_broadcast: %v", err), nil)
		}
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("cidr_broadcast: %q is not a CIDR", s), nil)
		}
		return common.MakeUDFSuccessResult(lastAddress(prefix).String(), nil)
	})
}

// lastAddress returns the highest address in a prefix.
func lastAddress(prefix netip.Prefix) netip.Addr {
	base := prefix.Masked().Addr()
	bitLen := base.BitLen()
	hostBits := bitLen - prefix.Bits()
	baseInt := new(big.Int).SetBytes(base.AsSlice())
	hosts := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(hostBits)), big.NewInt(1))
	lastInt := new(big.Int).Or(baseInt, hosts)
	bytes := lastInt.Bytes()
	if bitLen == 32 {
		var b [4]byte
		copy(b[4-len(bytes):], bytes)
		return netip.AddrFrom4(b)
	}
	var b [16]byte
	copy(b[16-len(bytes):], bytes)
	return netip.AddrFrom16(b)
}

// RegisterIPAdd registers ip_add, an address shifted by n (which may be
// negative). The result wraps within the address family.
func RegisterIPAdd() gojq.CompilerOption {
	return common.WithFunction("ip_add", 1, 1, func(v any, args []any) any {
		n, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("ip_add: expected an integer offset, got %v", args[0]), nil)
		}
		ip, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("ip_add: expected an IP string, got %T", v), nil)
		}
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ip_add: %q is not an address", ip), nil)
		}
		bitLen := addr.BitLen()
		value := new(big.Int).SetBytes(addr.AsSlice())
		value.Add(value, big.NewInt(int64(n)))
		mod := new(big.Int).Lsh(big.NewInt(1), uint(bitLen))
		value.Mod(value, mod)
		bytes := value.Bytes()
		if bitLen == 32 {
			var b [4]byte
			copy(b[4-len(bytes):], bytes)
			return common.MakeUDFSuccessResult(netip.AddrFrom4(b).String(), nil)
		}
		var b [16]byte
		copy(b[16-len(bytes):], bytes)
		return common.MakeUDFSuccessResult(netip.AddrFrom16(b).String(), nil)
	})
}

// RegisterIPv6Expand registers ipv6_expand, an IPv6 address in full
// eight-group form.
func RegisterIPv6Expand() gojq.CompilerOption {
	return common.WithFunction("ipv6_expand", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "ipv6_expand")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ipv6_expand: %v", err), nil)
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("ipv6_expand: %q is not an address", s), nil)
		}
		if addr.Is4() {
			return common.MakeUDFErrorResult(fmt.Errorf("ipv6_expand: %q is an IPv4 address", s), nil)
		}
		bytes := addr.As16()
		groups := make([]string, 8)
		for i := 0; i < 8; i++ {
			groups[i] = fmt.Sprintf("%04x", uint16(bytes[2*i])<<8|uint16(bytes[2*i+1]))
		}
		return common.MakeUDFSuccessResult(strings.Join(groups, ":"), nil)
	})
}

// RegisterReverseIP registers reverse_ip, the PTR record name for an address.
func RegisterReverseIP() gojq.CompilerOption {
	return common.WithFunction("reverse_ip", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "reverse_ip")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("reverse_ip: %v", err), nil)
		}
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("reverse_ip: %q is not an address", s), nil)
		}
		if addr.Is4() {
			bytes := addr.As4()
			return common.MakeUDFSuccessResult(fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa.",
				bytes[3], bytes[2], bytes[1], bytes[0]), nil)
		}
		bytes := addr.As16()
		var b strings.Builder
		for i := len(bytes) - 1; i >= 0; i-- {
			fmt.Fprintf(&b, "%x.%x.", bytes[i]&0x0f, bytes[i]>>4)
		}
		b.WriteString("ip6.arpa.")
		return common.MakeUDFSuccessResult(b.String(), nil)
	})
}

// RegisterSubnetOf registers subnet_of, whether one CIDR block is inside
// another.
func RegisterSubnetOf() gojq.CompilerOption {
	return common.WithFunction("subnet_of", 1, 1, func(v any, args []any) any {
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
	return common.WithFunction("cidr_first_host", 0, 2, func(v any, args []any) any {
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
	return common.WithFunction("cidr_last_host", 0, 2, func(v any, args []any) any {
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
	return common.WithFunction("port_name", 0, 0, func(v any, args []any) any {
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
