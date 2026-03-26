package main

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type nopConn struct{}

func (c *nopConn) Read(_ []byte) (int, error)       { return 0, io.EOF }
func (c *nopConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *nopConn) Close() error                     { return nil }
func (c *nopConn) LocalAddr() net.Addr              { return &net.IPAddr{} }
func (c *nopConn) RemoteAddr() net.Addr             { return &net.IPAddr{} }
func (c *nopConn) SetDeadline(time.Time) error      { return nil }
func (c *nopConn) SetReadDeadline(time.Time) error  { return nil }
func (c *nopConn) SetWriteDeadline(time.Time) error { return nil }

func TestSafeTransportDialsResolvedIPInsteadOfOriginalHostname(t *testing.T) {
	origLookup := alertLookupIPAddrs
	origDial := alertDialResolvedAddress
	defer func() {
		alertLookupIPAddrs = origLookup
		alertDialResolvedAddress = origDial
	}()

	alertLookupIPAddrs = func(ctx context.Context, host string) ([]net.IP, error) {
		if host != "alerts.example.test" {
			t.Fatalf("unexpected host lookup %q", host)
		}
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}

	var gotAddress string
	alertDialResolvedAddress = func(ctx context.Context, network, address string) (net.Conn, error) {
		gotAddress = address
		return &nopConn{}, nil
	}

	tr := safeTransport()
	conn, err := tr.DialContext(context.Background(), "tcp", "alerts.example.test:443")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = conn.Close()
	if gotAddress != "8.8.8.8:443" {
		t.Fatalf("expected dial to resolved IP, got %q", gotAddress)
	}
}

func TestSafeTransportRejectsPrivateResolvedIPs(t *testing.T) {
	origLookup := alertLookupIPAddrs
	defer func() { alertLookupIPAddrs = origLookup }()

	alertLookupIPAddrs = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}

	tr := safeTransport()
	_, err := tr.DialContext(context.Background(), "tcp", "alerts.example.test:443")
	if err == nil || !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("expected private IP rejection, got %v", err)
	}
}
