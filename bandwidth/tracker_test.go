package bandwidth

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// mockConn implements net.Conn for testing only the tracking layer.
type mockConn struct {
	readData  []byte
	writeBuf  []byte
	readPos   int
	closed    bool
	readErr   error
	writeErr  error
	mu        sync.Mutex
}

func newMockConn(data []byte) *mockConn {
	return &mockConn{readData: data, writeBuf: make([]byte, 0, 1024)}
}

func (m *mockConn) Read(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, net.ErrClosed
	}
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n := copy(b, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockConn) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, net.ErrClosed
	}
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.writeBuf = append(m.writeBuf, b...)
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestTracker_Reset(t *testing.T) {
	tracker := NewTracker()
	conn := newMockConn([]byte("hello"))
	tracked := tracker.TrackConnection(context.Background(), conn)

	// read to accumulate bytes
	buf := make([]byte, 5)
	n, err := tracked.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes, got %d", n)
	}

	if got := tracker.GetTotalBandwidth(); got == 0 {
		t.Fatal("expected non-zero bandwidth after read")
	}

	tracker.Reset()
	if got := tracker.GetTotalBandwidth(); got != 0 {
		t.Fatalf("expected 0 after reset, got %d", got)
	}
	if got := tracker.GetReadBytes(); got != 0 {
		t.Fatalf("expected 0 read bytes after reset, got %d", got)
	}
	if got := tracker.GetWriteBytes(); got != 0 {
		t.Fatalf("expected 0 write bytes after reset, got %d", got)
	}
}

func TestTracker_ReadTracking(t *testing.T) {
	tracker := NewTracker()
	conn := newMockConn([]byte("hello world"))
	tracked := tracker.TrackConnection(context.Background(), conn)

	buf := make([]byte, 5)
	n, err := tracked.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes, got %d", n)
	}
	if got := tracker.GetReadBytes(); got != 5 {
		t.Fatalf("expected 5 read bytes, got %d", got)
	}

	// read remaining
	n, err = tracked.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes, got %d", n)
	}
	if got := tracker.GetReadBytes(); got != 10 {
		t.Fatalf("expected 10 read bytes, got %d", got)
	}
}

func TestTracker_WriteTracking(t *testing.T) {
	tracker := NewTracker()
	conn := newMockConn(nil)
	tracked := tracker.TrackConnection(context.Background(), conn)

	data := []byte("hello world")
	n, err := tracked.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != 11 {
		t.Fatalf("expected 11 bytes, got %d", n)
	}
	if got := tracker.GetWriteBytes(); got != 11 {
		t.Fatalf("expected 11 write bytes, got %d", got)
	}
}

func TestTracker_TotalBandwidth(t *testing.T) {
	tracker := NewTracker()
	conn := newMockConn([]byte("abcdefghij"))
	tracked := tracker.TrackConnection(context.Background(), conn)

	// read 4 bytes
	buf := make([]byte, 4)
	n, err := tracked.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("expected 4 bytes, got %d", n)
	}

	// write 6 bytes
	data := []byte("123456")
	n, err = tracked.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("expected 6 bytes, got %d", n)
	}

	if got := tracker.GetTotalBandwidth(); got != 10 {
		t.Fatalf("expected 10 total bytes, got %d", got)
	}
}

func TestTracker_ReadError(t *testing.T) {
	tracker := NewTracker()
	conn := newMockConn(nil)
	conn.readErr = errors.New("read failure")
	tracked := tracker.TrackConnection(context.Background(), conn)

	buf := make([]byte, 10)
	n, err := tracked.Read(buf)
	if err == nil || err.Error() != "read failure" {
		t.Fatalf("expected 'read failure', got %v", err)
	}
	// error should not increment counter
	if n != 0 {
		t.Fatalf("expected 0 bytes on error, got %d", n)
	}
	if tracker.GetReadBytes() != 0 {
		t.Fatalf("expected 0 read bytes on error, got %d", tracker.GetReadBytes())
	}
}

func TestTracker_WriteError(t *testing.T) {
	tracker := NewTracker()
	conn := newMockConn(nil)
	conn.writeErr = errors.New("write failure")
	tracked := tracker.TrackConnection(context.Background(), conn)

	data := []byte("hello")
	n, err := tracked.Write(data)
	if err == nil || err.Error() != "write failure" {
		t.Fatalf("expected 'write failure', got %v", err)
	}
	// error should not increment counter
	if n != 0 {
		t.Fatalf("expected 0 bytes on error, got %d", n)
	}
	if tracker.GetWriteBytes() != 0 {
		t.Fatalf("expected 0 write bytes on error, got %d", tracker.GetWriteBytes())
	}
}

func TestTracker_TrackConnectionReturnsBTConn(t *testing.T) {
	tracker := NewTracker()
	conn := newMockConn(nil)
	tracked := tracker.TrackConnection(context.Background(), conn)

	if _, ok := tracked.(*BTConn); !ok {
		t.Fatalf("expected *BTConn, got %T", tracked)
	}
}

func TestTracker_InterfaceCompliance(t *testing.T) {
	var _ BandwidthTracker = NewTracker()
}

func TestNopeTracker_InterfaceCompliance(t *testing.T) {
	var _ BandwidthTracker = NewNopeTracker()
}

func TestNopeTracker_AllMethodsReturnZero(t *testing.T) {
	nt := NewNopeTracker()
	if got := nt.GetReadBytes(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := nt.GetWriteBytes(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := nt.GetTotalBandwidth(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestNopeTracker_TrackConnectionPassesThrough(t *testing.T) {
	nt := NewNopeTracker()
	conn := newMockConn([]byte("hello"))
	tracked := nt.TrackConnection(context.Background(), conn)

	// should be the exact same conn, not wrapped
	if tracked != conn {
		t.Fatalf("NopeTracker should return the original conn, got %T", tracked)
	}
}

func TestNopeTracker_ResetIsNoop(t *testing.T) {
	nt := NewNopeTracker()
	nt.Reset() // should not panic
	if nt.GetTotalBandwidth() != 0 {
		t.Fatal("NopeTracker should always return 0")
	}
}

func TestTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewTracker()
	conn := newMockConn(make([]byte, 1000))
	tracked := tracker.TrackConnection(context.Background(), conn)

	var wg sync.WaitGroup
	readers := 10
	writers := 10
	iterations := 100

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 10)
			for j := 0; j < iterations; j++ {
				tracked.Read(buf)
			}
		}()
	}

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := []byte("0123456789")
			for j := 0; j < iterations; j++ {
				tracked.Write(data)
			}
		}()
	}

	wg.Wait()

	// total should be non-zero (at minimum, writes succeeded)
	total := tracker.GetTotalBandwidth()
	if total <= 0 {
		t.Fatal("expected non-zero total bandwidth after concurrent access")
	}
	// verify read+write >= total (allow small discrepancies due to concurrent mock reads)
	if tracker.GetReadBytes()+tracker.GetWriteBytes() < total {
		t.Fatalf("read(%d)+write(%d) < total(%d)", tracker.GetReadBytes(), tracker.GetWriteBytes(), total)
	}
}
