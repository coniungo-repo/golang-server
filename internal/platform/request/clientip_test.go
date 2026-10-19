package request

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name, remote, header, value, trusted, want string
	}{
		{"socket", "203.0.113.7:5000", "", "", "", "203.0.113.7"},
		{"ipv6 socket", "[2001:db8::1]:5000", "", "", "", "2001:db8::1"},
		{"mapped ipv4", "[::ffff:203.0.113.7]:5000", "", "", "", "203.0.113.7"},
		{"untrusted header ignored", "203.0.113.7:5000", "Fly-Client-IP", "198.51.100.1", "", "203.0.113.7"},
		{"trusted header", "10.0.0.1:5000", "Fly-Client-IP", "198.51.100.1", "Fly-Client-IP", "198.51.100.1"},
		{"trusted header with spaces", "10.0.0.1:5000", "Fly-Client-IP", " 198.51.100.1 ", "Fly-Client-IP", "198.51.100.1"},
		{"list falls back", "10.0.0.1:5000", "Fly-Client-IP", "6.6.6.6, 198.51.100.1", "Fly-Client-IP", "10.0.0.1"},
		{"garbage header falls back", "10.0.0.1:5000", "Fly-Client-IP", "not-an-ip", "Fly-Client-IP", "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remote
			if tt.header != "" {
				r.Header.Set(tt.header, tt.value)
			}
			if got := ClientIP(r, tt.trusted); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientIPIgnoresRepeatedHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5000"
	r.Header.Add("Fly-Client-IP", "6.6.6.6")
	r.Header.Add("Fly-Client-IP", "198.51.100.1")
	if got := ClientIP(r, "Fly-Client-IP"); got != "10.0.0.1" {
		t.Fatalf("got %q, want the socket address", got)
	}
}
