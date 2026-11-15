// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gitea.dev/modules/util/rotatingfilewriter"
)

// WriterOpener describes a log destination. WriterKey identifies the target
// itself (file path, address, stream), not its settings: two openers with the
// same key share one writer, Caddy-style, so file rotation state or a network
// connection can never diverge between sinks. Settings divergence is detected
// via the opener's fields and reported once.
type WriterOpener interface {
	fmt.Stringer
	WriterKey() string
	OpenWriter() (io.WriteCloser, error)
}

type consoleWriterOpener struct {
	stderr bool
}

func (o consoleWriterOpener) String() string {
	if o.stderr {
		return "std:err"
	}
	return "std:out"
}

func (o consoleWriterOpener) WriterKey() string { return o.String() }

func (o consoleWriterOpener) OpenWriter() (io.WriteCloser, error) {
	if o.stderr {
		return nopWriteCloser{os.Stderr}, nil
	}
	return nopWriteCloser{os.Stdout}, nil
}

type fileWriterOpener struct {
	opts FileSinkOptions
}

func (o fileWriterOpener) String() string { return o.WriterKey() }

func (o fileWriterOpener) WriterKey() string { return "file:" + filepath.Clean(o.opts.FileName) }

func (o fileWriterOpener) OpenWriter() (io.WriteCloser, error) {
	w, err := rotatingfilewriter.Open(o.opts.FileName, &rotatingfilewriter.Options{
		Rotate:           o.opts.LogRotate,
		MaximumSize:      o.opts.MaxSize,
		RotateDaily:      o.opts.DailyRotate,
		KeepDays:         o.opts.MaxDays,
		Compress:         o.opts.Compress,
		CompressionLevel: o.opts.CompressionLevel,
	})
	if err != nil {
		// same trade-off as modules/log: a broken file sink degrades to
		// stderr rather than disabling logging entirely
		FallbackErrorf("unable to open log file %q: %v", o.opts.FileName, err)
		return nopWriteCloser{os.Stderr}, nil
	}
	return w, nil
}

type connWriterOpener struct {
	opts ConnSinkOptions
}

func newConnWriterOpener(cfg SinkConfig) connWriterOpener {
	opt := cfg.Conn
	if opt == nil {
		opt = &ConnSinkOptions{}
	}
	protocol := opt.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	dialTimeout, writeTimeout := opt.DialTimeout, opt.WriteTimeout
	if dialTimeout == 0 {
		dialTimeout = 30 * time.Second
	}
	if writeTimeout == 0 {
		writeTimeout = 60 * time.Second
	}
	return connWriterOpener{opts: ConnSinkOptions{
		Addr:         opt.Addr,
		Protocol:     protocol,
		DialTimeout:  dialTimeout,
		WriteTimeout: writeTimeout,
	}}
}

func (o connWriterOpener) String() string { return o.WriterKey() }

func (o connWriterOpener) WriterKey() string { return "net:" + o.opts.Protocol + "//" + o.opts.Addr }

func (o connWriterOpener) OpenWriter() (io.WriteCloser, error) {
	return &connWriter{
		protocol:     o.opts.Protocol,
		addr:         o.opts.Addr,
		dialTimeout:  o.opts.DialTimeout,
		writeTimeout: o.opts.WriteTimeout,
	}, nil
}

// sinkWriterOpener returns the opener describing cfg's destination, or nil for
// an unknown sink type (rejected by Config validation before this is called).
func sinkWriterOpener(cfg SinkConfig) WriterOpener {
	switch cfg.Type {
	case "console":
		opt := cfg.Console
		if opt == nil {
			opt = &ConsoleSinkOptions{}
		}
		return consoleWriterOpener{stderr: opt.Stderr}
	case "file":
		opt := cfg.File
		if opt == nil {
			opt = &FileSinkOptions{}
		}
		return fileWriterOpener{opts: *opt}
	case "conn":
		return newConnWriterOpener(cfg)
	default:
		return nil
	}
}

// writerPool is a usage-counted registry of open writers keyed by target
// identity (the port of Caddy's WriterKey/UsagePool pair). Sinks acquire on
// build and release when they leave every logger tree; the writer closes when
// the last reference drops.
type writerPool struct {
	mu      sync.Mutex
	writers map[string]*pooledWriter
}

type pooledWriter struct {
	w      io.WriteCloser
	refs   int
	repr   string
	warned bool
}

// writerRepr renders an opener's settings for divergence detection. %#v skips
// the Stringer, so it shows the actual fields.
func writerRepr(op WriterOpener) string {
	return fmt.Sprintf("%#v", op)
}

// acquire returns the writer for op's target, opening it on first use. The
// returned release func must be called exactly once, when the sink is no longer
// reachable from any tree. A second sink targeting an open writer keeps the existing
// settings; divergence from its own config is reported once.
func (p *writerPool) acquire(op WriterOpener) (io.WriteCloser, func(), error) {
	key, repr := op.WriterKey(), writerRepr(op)
	p.mu.Lock()
	defer p.mu.Unlock()
	if pw := p.writers[key]; pw != nil {
		if pw.repr != repr && !pw.warned {
			pw.warned = true
			FallbackErrorf("log writer %q is already open with different settings, keeping the existing ones", key)
		}
		pw.refs++
		w := pw.w
		return w, func() { p.release(key) }, nil
	}
	w, err := op.OpenWriter()
	if err != nil {
		return nil, nil, err
	}
	if p.writers == nil {
		p.writers = map[string]*pooledWriter{}
	}
	p.writers[key] = &pooledWriter{w: w, refs: 1, repr: repr}
	return w, func() { p.release(key) }, nil
}

func (p *writerPool) release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pw := p.writers[key]
	if pw == nil {
		return // defensive: every acquire is paired with exactly one release
	}
	pw.refs--
	if pw.refs <= 0 {
		delete(p.writers, key)
		_ = pw.w.Close()
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }
