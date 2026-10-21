package valkeytest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// Proxy passes connections through to Valkey, and holds back the reply to the first SET ... NX it sees until its client
// hangs up: the lock is then taken while its taker hears nothing, however long the taker would wait.
type Proxy struct {
	URL string
	// LockAttempts counts the SET ... NX commands passed through.
	LockAttempts atomic.Int32
	upstream     string
}

// StartProxy listens on a port of its own in front of the Valkey at upstream.
func StartProxy(t *testing.T, upstream string) *Proxy {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxy := &Proxy{URL: "redis://" + listener.Addr().String(), upstream: strings.TrimPrefix(upstream, "redis://")}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Logf("the proxy did not stop: %v", err)
		}
	})
	go proxy.serve(listener)
	return proxy
}

func (p *Proxy) serve(listener net.Listener) {
	for {
		client, err := listener.Accept()
		if err != nil {
			return
		}
		go p.pass(client)
	}
}

// pass forwards one connection both ways; the reply after the first SET NX waits for the client to hang up.
func (p *Proxy) pass(client net.Conn) {
	server, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", p.upstream)
	if err != nil {
		closeQuietly(client)
		return
	}
	var holdReply atomic.Bool
	hungUp := make(chan struct{})
	go func() {
		defer close(hungUp)
		defer closeQuietly(server)
		copyChunks(server, client, func(chunk []byte) {
			if isSetNX(chunk) && p.LockAttempts.Add(1) == 1 {
				holdReply.Store(true)
			}
		})
	}()
	defer closeQuietly(client)
	copyChunks(client, server, func([]byte) {
		if holdReply.CompareAndSwap(true, false) {
			<-hungUp
		}
	})
}

// copyChunks copies from one side to the other until either fails, showing each chunk to before first.
func copyChunks(to io.Writer, from io.Reader, before func([]byte)) {
	buf := make([]byte, 64<<10)
	for {
		n, err := from.Read(buf)
		if n > 0 {
			before(buf[:n])
			if _, err := to.Write(buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// isSetNX matches go-redis's SET key value PX ttl NX.
func isSetNX(command []byte) bool {
	lower := bytes.ToLower(command)
	return bytes.Contains(lower, []byte("\r\nset\r\n")) && bytes.HasSuffix(lower, []byte("\r\nnx\r\n"))
}

// closeQuietly closes a side of a connection the other side may have closed first, which is how one ends here; a test
// proxy has nobody to tell of another failure.
func closeQuietly(conn io.Closer) {
	if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return
	}
}
