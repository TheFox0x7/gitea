// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"fmt"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"strings"
)

var projectPackagePrefix string

func init() {
	_, filename, _, _ := runtime.Caller(0)
	projectPackagePrefix = strings.TrimSuffix(filename, "modules/slog/internal.go")
	if projectPackagePrefix == filename {
		panic("unable to detect correct package prefix, please update file: " + filename)
	}
}

// FallbackErrorf is the last-resort output when the logging system itself fails
// (e.g. its writer errors).
func FallbackErrorf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// OsExiter is the exit function used after a Fatal log, overridable for tests.
var OsExiter = os.Exit

type LogStringer interface {
	LogString() string
}

func asLogStringer(v any) LogStringer {
	if s, ok := v.(LogStringer); ok {
		return s
	} else if a := reflect.ValueOf(v); a.Kind() == reflect.Struct {
		// in case the receiver is a pointer, but the value is a struct
		vp := reflect.New(a.Type())
		vp.Elem().Set(a)
		if s, ok := reflect.TypeAssert[LogStringer](vp); ok {
			return s
		}
	}
	return nil
}

type logStringFormatter struct {
	v LogStringer
}

var _ fmt.Formatter = logStringFormatter{}

func (l logStringFormatter) Format(f fmt.State, verb rune) {
	if f.Flag('#') && verb == 'v' {
		_, _ = fmt.Fprintf(f, "%#v", l.v)
		return
	}
	_, _ = f.Write([]byte(l.v.LogString()))
}

// protectSensitiveInfo masks userinfo and query values of http(s) URLs so they
// never reach log output. It reports whether the string changed.
func protectSensitiveInfo(s string) (string, bool) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return s, false
	}
	m := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	changed := false
	if u.User != nil {
		m.User = url.User("_masked_")
		changed = true
	}
	q := u.Query()
	for _, vals := range q {
		for i := range vals {
			vals[i] = "_"
			changed = true
		}
	}
	if len(q) > 0 {
		m.RawQuery = q.Encode()
	}
	if !changed {
		return s, false
	}
	return m.String(), true
}

// Copy of cheap integer to fixed-width decimal to ascii from the stdlib logger.
func itoa(buf []byte, i, wid int) []byte {
	var s [20]byte
	bp := len(s) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		s[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	s[bp] = byte('0' + i)
	return append(buf, s[bp:]...)
}

func colorSprintf(colorize bool, format string, args ...any) string {
	hasColorValue := false
	for _, v := range args {
		if _, hasColorValue = v.(*ColoredValue); hasColorValue {
			break
		}
	}
	if colorize || !hasColorValue {
		return fmt.Sprintf(format, args...)
	}

	noColors := make([]any, len(args))
	copy(noColors, args)
	for i, v := range args {
		if cv, ok := v.(*ColoredValue); ok {
			noColors[i] = cv.v
		}
	}
	return fmt.Sprintf(format, noColors...)
}
