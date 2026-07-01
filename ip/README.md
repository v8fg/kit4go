# ip: CIDR & mask utilities

Parse CIDR notation and convert between mask representations. Pure standard
library. `AddrLookup` mirrors the `net.InterfaceAddrs` / `net.Interfaces`
signatures so local-interface enumeration is mockable in tests.

## Usage

- `ParseCIDR(cidr string) (Flag, string, *net.IPNet)` parse + version flag.
- `MaskByte(cidr string) []byte` mask as raw bytes.
- `MaskString(cidr string) string` mask as dotted-decimal.
- `CIDRToIPMask(cidr string) string` CIDR -> IP mask string.
- `MaskIPToCIDR(ipMask string) string` IP mask string -> CIDR.

`AddrLookup` interface: `InterfaceAddrs() ([]net.Addr, error)` and
`Interfaces() ([]net.Interface, error)`.

## Example

```go
import "github.com/v8fg/kit4go/ip"

flag, addr, ipnet, _ := ip.ParseCIDR("10.0.0.0/8")
_ = ipnet
_ = addr
m := ip.MaskString("10.0.0.0/8") // "255.0.0.0"
```
