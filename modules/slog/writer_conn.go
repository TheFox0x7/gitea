// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"io"
	"net"
	"sync"
	"time"
)

// connRedialInterval caps reconnect attempts so a down remote cannot turn
// every log emission into a blocking dial (the pathology of Caddy #4111).
var connRedialInterval = 10 * time.Second

// connWriter is a port of Caddy's redialerConn: a failed write retries once
// (a concurrent writer may have reconnected already), redials are
// rate-limited, and while the remote is down writes fail fast so the sink can
// dump the line to stderr. Unlike Caddy, writes carry a deadline, so a
// stalled reader fails into the same redial path instead of blocking forever.
type connWriter struct {
	connMu      sync.RWMutex
	innerWriter net.Conn

	protocol     string
	addr         string
	dialTimeout  time.Duration
	writeTimeout time.Duration

	lastRedial time.Time
	lastError  error
}

var _ io.WriteCloser = (*connWriter)(nil)

func (c *connWriter) Close() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.innerWriter != nil {
		err := c.innerWriter.Close()
		c.innerWriter = nil
		return err
	}
	return nil
}

func (c *connWriter) Write(p []byte) (int, error) {
	c.connMu.RLock()
	conn := c.innerWriter
	c.connMu.RUnlock()
	if conn != nil {
		if n, err := c.writeConn(conn, p); err == nil {
			return n, nil
		}
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()
	// a concurrent write may have reconnected while we waited for the lock
	if c.innerWriter != nil {
		if n, err := c.writeConn(c.innerWriter, p); err == nil {
			return n, nil
		}
	}
	if time.Since(c.lastRedial) >= connRedialInterval {
		conn, err := c.dial()
		if err == nil {
			if c.innerWriter != nil {
				_ = c.innerWriter.Close()
			}
			c.innerWriter = conn
			return c.writeConn(conn, p)
		}
	}
	return 0, c.lastError
}

func (c *connWriter) dial() (net.Conn, error) {
	c.lastRedial = time.Now()
	conn, err := net.DialTimeout(c.protocol, c.addr, c.dialTimeout)
	if err != nil {
		c.lastError = err
		return nil, err
	}
	return conn, nil
}

func (c *connWriter) writeConn(conn net.Conn, p []byte) (int, error) {
	if c.writeTimeout > 0 {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
		}
	}
	n, err := conn.Write(p)
	if err != nil {
		c.lastError = err
	}
	return n, err
}
