package connector

// Pigeonhole: this file isn't in upstream mautrix-meta.
//
// After the computer sleeps, the encrypted chats' connection is usually dead
// without knowing it. Resetting it closes it politely first, which waits on
// the dead connection for 5 to 25 seconds before reconnecting. Tracking the
// connection lets a reset cut it off instead, so it reconnects at once.

import (
	"context"
	"net"
	"net/http"
	"sync"
)

// connTracker remembers the network connections a dialer has open.
type connTracker struct {
	lock  sync.Mutex
	conns map[*trackedConn]struct{}
}

type trackedConn struct {
	net.Conn
	tracker *connTracker
	once    sync.Once
}

func (c *trackedConn) Close() error {
	c.once.Do(func() {
		c.tracker.lock.Lock()
		delete(c.tracker.conns, c)
		c.tracker.lock.Unlock()
	})
	return c.Conn.Close()
}

// transport gives an HTTP transport whose connections are tracked.
func (t *connTracker) transport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dial := transport.DialContext
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		tracked := &trackedConn{Conn: conn, tracker: t}
		t.lock.Lock()
		if t.conns == nil {
			t.conns = make(map[*trackedConn]struct{})
		}
		t.conns[tracked] = struct{}{}
		t.lock.Unlock()
		return tracked, nil
	}
	return transport
}

// closeAll cuts off every open connection, and says whether there were any.
func (t *connTracker) closeAll() bool {
	t.lock.Lock()
	conns := make([]*trackedConn, 0, len(t.conns))
	for conn := range t.conns {
		conns = append(conns, conn)
	}
	t.lock.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
	return len(conns) > 0
}
