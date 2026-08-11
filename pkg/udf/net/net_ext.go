package net

import (
	"fmt"
	"math/big"
	"net/netip"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterIPVersion registers ip_version, "v4" or "v6" for an address.
func RegisterIPVersion() gojq.CompilerOption {
	return gojq.WithFunction("ip_version", 0, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("cidr_network", 0, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("cidr_broadcast", 0, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("ip_add", 1, 1, func(v any, args []any) any {
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
	return gojq.WithFunction("ipv6_expand", 0, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("reverse_ip", 0, 2, func(v any, args []any) any {
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
