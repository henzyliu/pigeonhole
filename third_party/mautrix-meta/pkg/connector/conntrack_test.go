package connector

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

// Cutting connections off ends reads on them at once, and forgets them.
func TestConnTrackerCloseAll(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()
	var tracker connTracker
	if tracker.closeAll() {
		t.Fatal("nothing to close yet")
	}
	conn, err := tracker.transport().DialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 1))
		done <- err
	}()
	if !tracker.closeAll() {
		t.Fatal("open connection not closed")
	}
	select {
	case err := <-done:
		if err == nil || err == io.EOF {
			t.Fatalf("read ended with %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read still waiting after closing")
	}
	if tracker.closeAll() {
		t.Fatal("closed connection still tracked")
	}
}
