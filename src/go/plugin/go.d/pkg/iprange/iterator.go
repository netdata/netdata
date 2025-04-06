// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"iter"
	"net"
)

func iterate(r Range) iter.Seq[net.IP] {
	ipCopy := make(net.IP, len(r.getStart()))
	nextBuf := make(net.IP, len(r.getStart()))

	return func(yield func(net.IP) bool) {
		for ip := r.getStart(); ip != nil; ip = nextIP(ip, nextBuf) {
			copy(ipCopy, ip)
			if !yield(ipCopy) {
				return
			}
			if ip.Equal(r.getEnd()) {
				break
			}
		}
	}
}

func nextIP(ip net.IP, buf net.IP) net.IP {
	ip = ip.To16()
	if ip == nil {
		return nil
	}
	copy(buf, ip)

	for i := len(buf) - 1; i >= 0; i-- {
		buf[i]++
		if buf[i] != 0 {
			break
		}
	}
	return buf
}
