package request

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP returns the caller's IP. trustedHeader names a single-address header that a proxy you control overwrites (e.g. Fly-Client-IP); leave it empty to use the socket address. A header holding a list, or sent more than once, is ignored: in X-Forwarded-For the first entry is whatever the client sent.
func ClientIP(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		if vs := r.Header.Values(trustedHeader); len(vs) == 1 && !strings.Contains(vs[0], ",") {
			if addr, err := netip.ParseAddr(strings.TrimSpace(vs[0])); err == nil {
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
