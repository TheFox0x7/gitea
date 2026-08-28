// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
)

var unknown = []byte("???")

// Stack skips back the provided number of frames and returns a stack trace
// with source code. A skip of 0 returns the stack trace for the calling
// function, not including this call.
func Stack(skip int) string {
	buf := new(bytes.Buffer)

	// Store the last file we opened as it's probable that the preceding stack
	// frame will be in the same file
	var lines [][]byte
	var lastFilename string
	for i := skip + 1; ; i++ {
		programCounter, filename, lineNumber, ok := runtime.Caller(i)
		if !ok {
			break
		}

		_, _ = fmt.Fprintf(buf, "%s:%d (0x%x)\n", filename, lineNumber, programCounter)
		if filename != lastFilename {
			data, err := os.ReadFile(filename)
			if err != nil {
				continue
			}
			lines = bytes.Split(data, []byte{'\n'})
			lastFilename = filename
		}
		_, _ = fmt.Fprintf(buf, "\t%s: %s\n", functionName(programCounter), source(lines, lineNumber))
	}
	return buf.String()
}

func functionName(programCounter uintptr) []byte {
	function := runtime.FuncForPC(programCounter)
	if function == nil {
		return unknown
	}
	name := []byte(function.Name())

	if lastslash := bytes.LastIndex(name, []byte("/")); lastslash >= 0 {
		name = name[lastslash+1:]
	}
	if period := bytes.Index(name, []byte(".")); period >= 0 {
		name = name[period+1:]
	}
	name = bytes.ReplaceAll(name, []byte("·"), []byte("."))
	return name
}

func source(lines [][]byte, n int) []byte {
	n-- // in stack traces, lines are 1-indexed but our array is 0-indexed
	if n < 0 || n >= len(lines) {
		return unknown
	}
	return bytes.TrimSpace(lines[n])
}
