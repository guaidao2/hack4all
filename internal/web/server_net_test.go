package web

import (
	"net"
	"testing"
)

func TestBindIsLocal(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"[::1]:8080", true},
		{"0.0.0.0:8080", false},
		{"[::]:8080", false},
		{"192.168.10.5:8080", false},
	}

	for _, c := range cases {
		addr, err := net.ResolveTCPAddr("tcp", c.addr)
		if err != nil {
			t.Fatalf("resolve %s: %v", c.addr, err)
		}
		if got := bindIsLocal(addr); got != c.want {
			t.Errorf("bindIsLocal(%s) = %v, want %v", c.addr, got, c.want)
		}
	}
}

// A nil address must not panic: Serve is sometimes called with a listener whose
// address has already been closed.
func TestBindIsLocalHandlesNil(t *testing.T) {
	if bindIsLocal(nil) {
		t.Error("bindIsLocal(nil) = true, want false")
	}
}
