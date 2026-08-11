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
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
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
	return gojq.WithFunction("ip_to_int", 0, 2, func(v any, args []any) any {
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
			return common.MakeUDFSuccessResult(uint32(bytes[0])<<24 | uint32(bytes[1])<<16 | uint32(bytes[2])<<8 | uint32(bytes[3]), nil)
		}
		bytes := addr.As16()
		n := new(big.Int).SetBytes(bytes[:])
		return common.MakeUDFSuccessResult(n.String(), nil)
	})
}

// RegisterIntToIP registers int_to_ip, a decimal integer back to an address.
// Values up to 2^32-1 become IPv4; larger values become IPv6.
func RegisterIntToIP() gojq.CompilerOption {
	return gojq.WithFunction("int_to_ip", 0, 1, func(v any, args []any) any {
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
	return gojq.WithFunction("in_cidr", 1, 1, func(v any, args []any) any {
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
	return gojq.WithFunction("cidr_size", 0, 2, func(v any, args []any) any {
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
			return common.MakeUDFSuccessResult(int64(1) << uint(hostBits), nil)
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
	return gojq.WithFunction("mac_normalize", 0, 2, func(v any, args []any) any {
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
