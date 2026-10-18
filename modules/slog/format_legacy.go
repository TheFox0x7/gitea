// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"fmt"
	"log/slog"
	"strings"
)

// formatLegacy renders a record in the legacy Gitea text format. It is a port
// of modules/log's EventFormatTextMessage, adapted to consume slog.Record:
// caller/file/line come from Record.Source() (or hiddenInfo prefill), the
// message is the preformatted Record.Message, and colorized re-rendering uses
// hiddenInfo's printf format+args. Structured record attrs and the frontend's
// With-state render as a " key=value" suffix.
func (st *sinkState) formatLegacy(rec *slog.Record, fe *sinkFrontend, hi *hiddenInfo) []byte {
	flags := st.flags.Bits()
	colorize := st.colorize

	buf := make([]byte, 0, 1024)
	buf = append(buf, st.prefix...)
	t := rec.Time
	if flags&(Ldate|Ltime|Lmicroseconds) != 0 {
		if colorize {
			buf = append(buf, fgCyanBytes...)
		}
		if flags&LUTC != 0 {
			t = t.UTC()
		}
		if flags&Ldate != 0 {
			year, month, day := t.Date()
			buf = itoa(buf, year, 4)
			buf = append(buf, '/')
			buf = itoa(buf, int(month), 2)
			buf = append(buf, '/')
			buf = itoa(buf, day, 2)
			buf = append(buf, ' ')
		}
		if flags&(Ltime|Lmicroseconds) != 0 {
			hour, minNum, sec := t.Clock()
			buf = itoa(buf, hour, 2)
			buf = append(buf, ':')
			buf = itoa(buf, minNum, 2)
			buf = append(buf, ':')
			buf = itoa(buf, sec, 2)
			if flags&Lmicroseconds != 0 {
				buf = append(buf, '.')
				buf = itoa(buf, t.Nanosecond()/1e3, 6)
			}
			buf = append(buf, ' ')
		}
		if colorize {
			buf = append(buf, resetBytes...)
		}
	}

	caller, file, line := sourceParts(rec, hi)
	if flags&(Lshortfile|Llongfile) != 0 && file != "" {
		if colorize {
			buf = append(buf, fgGreenBytes...)
		}
		if flags&Lmedfile == Lmedfile {
			fileLen := len(file)
			const softLimit = 20
			if fileLen > softLimit {
				slashIndex := strings.LastIndexByte(file[:fileLen-softLimit], '/')
				if slashIndex != -1 {
					file = ".../" + file[slashIndex+1:]
				}
			}
		} else if flags&Lshortfile != 0 {
			startIndex := strings.LastIndexByte(file, '/')
			if startIndex > 0 && startIndex < len(file) {
				file = file[startIndex+1:]
			}
		}
		buf = append(buf, file...)
		buf = append(buf, ':')
		buf = itoa(buf, line, -1)
		if flags&(Lfuncname|Lshortfuncname) != 0 {
			buf = append(buf, ':')
		} else {
			if colorize {
				buf = append(buf, resetBytes...)
			}
			buf = append(buf, ' ')
		}
	}
	if flags&(Lfuncname|Lshortfuncname) != 0 && caller != "" {
		if colorize {
			buf = append(buf, fgGreenBytes...)
		}
		funcName := caller
		shortFuncName := funcName
		if flags&Lshortfuncname != 0 {
			// funcName = "gitea.dev/modules/foo/bar.MyFunc.func1.2()"
			slashPos := strings.LastIndexByte(funcName, '/')
			dotPos := strings.IndexByte(funcName[slashPos+1:], '.')
			if dotPos > 0 {
				// shortFuncName = "MyFunc.func1.2()"
				shortFuncName = funcName[slashPos+1+dotPos+1:]
				if strings.Contains(shortFuncName, ".") {
					shortFuncName = strings.ReplaceAll(shortFuncName, ".func", ".")
				}
			}
			funcName = shortFuncName
		}
		buf = append(buf, funcName...)
		if colorize {
			buf = append(buf, resetBytes...)
		}
		buf = append(buf, ' ')
	}

	if flags&(Llevel|Llevelinitial) != 0 {
		level := strings.ToUpper(LevelName(rec.Level))
		if colorize {
			buf = append(buf, ColorBytes(levelColorAttrs(rec.Level)...)...)
		}
		buf = append(buf, '[')
		if flags&Llevelinitial != 0 {
			buf = append(buf, level[0])
		} else {
			buf = append(buf, level...)
		}
		buf = append(buf, ']')
		if colorize {
			buf = append(buf, resetBytes...)
		}
		buf = append(buf, ' ')
	}

	var msg []byte

	// if the log needs colorizing, re-render from the printf format+args
	if colorize && hi != nil && len(hi.args) > 0 {
		hasColorValue := false
		for _, v := range hi.args {
			if _, hasColorValue = v.(*ColoredValue); hasColorValue {
				break
			}
		}
		if hasColorValue {
			msg = fmt.Appendf(nil, hi.format, hi.args...)
		}
	}
	if len(msg) == 0 {
		msg = []byte(rec.Message)
	}
	if len(msg) == 0 && hi != nil {
		msg = []byte(colorSprintf(colorize, hi.format, hi.args...))
	}
	// remove at most one trailing new line
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}

	if flags&Lgopid == Lgopid {
		deprecatedGoroutinePid := "no-gopid" // dummy value to avoid breaking the log format
		buf = append(buf, '[')
		if colorize {
			buf = append(buf, ColorBytes(FgHiYellow)...)
		}
		buf = append(buf, deprecatedGoroutinePid...)
		if colorize {
			buf = append(buf, resetBytes...)
		}
		buf = append(buf, ']', ' ')
	}
	buf = append(buf, msg...)

	buf = fe.appendLegacyAttrs(buf, rec)

	if hi != nil && len(hi.stack) > 0 && st.stacktraceLevel <= rec.Level {
		for line := range strings.SplitSeq(formatLegacyStack(hi.stack), "\n") {
			buf = append(buf, "\n\t"...)
			buf = append(buf, line...)
		}
		buf = append(buf, '\n')
	}
	buf = append(buf, '\n')
	return buf
}

func sourceParts(rec *slog.Record, hi *hiddenInfo) (caller, file string, line int) {
	if hi != nil && hi.caller != "" {
		return hi.caller, hi.file, hi.line
	}
	if src := rec.Source(); src != nil {
		return strings.ReplaceAll(src.Function, "[...]", "") + "()", strings.TrimPrefix(src.File, projectPackagePrefix), src.Line
	}
	return "", "", 0
}

// appendLegacyAttrs renders structured attrs as a " key=value" suffix in the
// legacy text format (frontend With-state first, then record attrs).
func (f *sinkFrontend) appendLegacyAttrs(buf []byte, rec *slog.Record) []byte {
	if len(f.withAttrs) == 0 && rec.NumAttrs() == 0 {
		return buf
	}
	groupPrefix := strings.Join(f.groups, ".")
	if groupPrefix != "" {
		groupPrefix += "."
	}
	for _, a := range f.withAttrs {
		buf = appendAttrSuffix(buf, groupPrefix, a)
	}
	rec.Attrs(func(a slog.Attr) bool {
		buf = appendAttrSuffix(buf, groupPrefix, a)
		return true
	})
	return buf
}

func appendAttrSuffix(buf []byte, groupPrefix string, a slog.Attr) []byte {
	buf = append(buf, ' ')
	buf = append(buf, groupPrefix...)
	buf = append(buf, a.Key...)
	buf = append(buf, '=')
	buf = append(buf, a.Value.String()...)
	return buf
}
