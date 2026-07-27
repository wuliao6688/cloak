package tls_client

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

type delayedProxyDialer struct {
	started chan struct{}
	release chan struct{}
	conn    net.Conn
}

func (d *delayedProxyDialer) Dial(string, string) (net.Conn, error) {
	close(d.started)
	<-d.release
	return d.conn, nil
}

func TestSocksContextDialerClosesLateConnectionAfterCancellation(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	underlying := &delayedProxyDialer{
		started: make(chan struct{}),
		release: make(chan struct{}),
		conn:    clientConn,
	}
	dialer := newSocksContextDialer(underlying)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := dialer.DialContext(ctx, "tcp", "example.com:443")
		result <- err
	}()

	<-underlying.started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected cancellation error: %v", err)
	}
	close(underlying.release)

	if err := serverConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
			return
		}
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	if _, err := serverConn.Read(buffer); err == nil {
		t.Fatal("late SOCKS connection was not closed after cancellation")
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatal("timed out waiting for the late SOCKS connection to close")
	}
}
