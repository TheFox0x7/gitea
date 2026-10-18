// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnWriterRedial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close()) // a known-dead target

	oldInterval := connRedialInterval
	connRedialInterval = 10 * time.Millisecond
	t.Cleanup(func() { connRedialInterval = oldInterval })

	w := &connWriter{protocol: "tcp", addr: addr, dialTimeout: time.Second, writeTimeout: time.Second}
	t.Cleanup(func() { _ = w.Close() })

	_, err = w.Write([]byte("first\n"))
	require.Error(t, err) // nothing listens: the dial fails and the error surfaces
	_, err = w.Write([]byte("second\n"))
	require.Error(t, err)
	assert.Same(t, err, w.lastError) // rate-limited: the retry reused the first dial's error

	// the remote comes back: writes succeed again once the redial window passed
	ln2, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	defer ln2.Close()
	received := make(chan string, 1)
	go func() {
		conn, err := ln2.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 128)
		n, _ := conn.Read(buf)
		received <- string(buf[:n])
	}()

	assert.Eventually(t, func() bool {
		_, err := w.Write([]byte("reconnected\n"))
		return err == nil
	}, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, "reconnected\n", <-received)
}
