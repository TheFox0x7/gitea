// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"io"
	"net"
	"time"
)

// connWriter is a port of modules/log's conn writer with dial/write deadlines
// added so an unresponsive peer cannot wedge the sink goroutine forever.
type connWriter struct {
	innerWriter io.WriteCloser

	protocol       string
	addr           string
	reconnect      bool
	reconnectOnMsg bool
	dialTimeout    time.Duration
	writeTimeout   time.Duration
}

var _ io.WriteCloser = (*connWriter)(nil)

func (c *connWriter) Close() error {
	if c.innerWriter != nil {
		return c.innerWriter.Close()
	}
	return nil
}

func (c *connWriter) Write(p []byte) (int, error) {
	if c.neededConnectOnMsg() {
		if err := c.connect(); err != nil {
			return 0, err
		}
	}

	if c.reconnectOnMsg {
		defer c.innerWriter.Close()
	}

	if c.writeTimeout > 0 {
		if tcpConn, ok := c.innerWriter.(*net.TCPConn); ok {
			_ = tcpConn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
		}
	}
	return c.innerWriter.Write(p)
}

func (c *connWriter) neededConnectOnMsg() bool {
	if c.reconnect {
		c.reconnect = false
		return true
	}
	if c.innerWriter == nil {
		return true
	}
	return c.reconnectOnMsg
}

func (c *connWriter) connect() error {
	if c.innerWriter != nil {
		_ = c.innerWriter.Close()
		c.innerWriter = nil
	}

	conn, err := net.DialTimeout(c.protocol, c.addr, c.dialTimeout)
	if err != nil {
		return err
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
	}

	c.innerWriter = conn
	return nil
}
