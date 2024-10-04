# iprange

This package helps you to work with IP ranges.

IP range is a set of IP addresses. Both IPv4 and IPv6 are supported.

IP range interface:

```
type Range interface {
	Family() Family
	Contains(ip net.IP) bool
	Size() *big.Int
	fmt.Stringer
}
```  

## Supported formats

- `IPv4 address` (192.0.2.1)
- `IPv4 range` (192.0.2.0-192.0.2.10)
- `IPv4 CIDR` (192.0.2.0/24)
- `IPv4 subnet mask` (192.0.2.0/255.255.255.0)
- `IPv6 address` (2001:db8::1)
- `IPv6 range` (2001:db8::-2001:db8::10)
- `IPv6 CIDR` (2001:db8::/64)

IP range doesn't contain network and broadcast IP addresses if the format is `IPv4 CIDR`, `IPv4 subnet mask`
or `IPv6 CIDR`.  
