package graphics

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestMediaDialRespectsRequestedAddressFamily(t *testing.T) {
	dns, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var ipv6Queries atomic.Int32
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		packet := make([]byte, 1024)
		for {
			n, peer, err := dns.ReadFrom(packet)
			if err != nil {
				return
			}
			end := 12
			for end < n && packet[end] != 0 {
				end += int(packet[end]) + 1
			}
			end++
			if end+4 > n {
				continue
			}
			kind := binary.BigEndian.Uint16(packet[end : end+2])
			response := append([]byte(nil), packet[:end+4]...)
			binary.BigEndian.PutUint16(response[2:4], 0x8180)
			clear(response[6:12])
			if kind == 1 {
				binary.BigEndian.PutUint16(response[6:8], 1)
				response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4, 127, 0, 0, 1)
			}
			if kind == 28 {
				ipv6Queries.Add(1)
			}
			_, _ = dns.WriteTo(response, peer)
		}
	}()
	t.Cleanup(func() { _ = dns.Close(); <-finished })
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", dns.LocalAddr().String())
	}}
	t.Cleanup(func() { net.DefaultResolver = original })
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	m := New("https://avatar.example.test:" + port)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	conn, err := m.dial(ctx, "tcp4", "avatar.example.test:"+port)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if ipv6Queries.Load() != 0 {
		t.Fatal("custom media resolution bypassed the dialer's requested address family")
	}
	m.origin.Host = "unrelated.example.test"
	if conn, err := m.dial(ctx, "tcp4", "avatar.example.test:"+port); err == nil {
		_ = conn.Close()
		t.Fatal("DNS-resolved private address bypassed the configured-origin restriction")
	}
}

func TestMediaDialRejectsPrivateAddressesOutsideConfiguredOrigin(t *testing.T) {
	m := New("https://example.test")
	for _, address := range []string{"127.0.0.1:443", "[::1]:443", "10.0.0.1:443", "169.254.169.254:443", "[fe80::1%lo]:443"} {
		t.Run(address, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			conn, err := m.dial(ctx, "tcp", address)
			if err == nil {
				_ = conn.Close()
				t.Fatal("private image destination accepted")
			}
		})
	}
}
