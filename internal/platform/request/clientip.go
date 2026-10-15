package request

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP returns the caller's IP. trustedHeader names a header set by a proxy you control (e.g. Fly-Client-IP); leave it empty to use the socket address.
func ClientIP(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		v := strings.TrimSpace(r.Header.Get(trustedHeader))
		if first, _, _ := strings.Cut(v, ","); first != "" {
			if addr, err := netip.ParseAddr(strings.TrimSpace(first)); err == nil {
				return addr.Unmap().String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return host
}
