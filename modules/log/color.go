// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"gitea.dev/modules/slog"
)

// ColorAttribute defines a single SGR color/attribute code.
type ColorAttribute = slog.ColorAttribute

// Base ColorAttributes
const (
	Reset        = slog.Reset
	Bold         = slog.Bold
	Faint        = slog.Faint
	Italic       = slog.Italic
	Underline    = slog.Underline
	BlinkSlow    = slog.BlinkSlow
	BlinkRapid   = slog.BlinkRapid
	ReverseVideo = slog.ReverseVideo
	Concealed    = slog.Concealed
	CrossedOut   = slog.CrossedOut
)

// Foreground text colors
const (
	FgBlack   = slog.FgBlack
	FgRed     = slog.FgRed
	FgGreen   = slog.FgGreen
	FgYellow  = slog.FgYellow
	FgBlue    = slog.FgBlue
	FgMagenta = slog.FgMagenta
	FgCyan    = slog.FgCyan
	FgWhite   = slog.FgWhite
)

// Foreground Hi-Intensity text colors
const (
	FgHiBlack   = slog.FgHiBlack
	FgHiRed     = slog.FgHiRed
	FgHiGreen   = slog.FgHiGreen
	FgHiYellow  = slog.FgHiYellow
	FgHiBlue    = slog.FgHiBlue
	FgHiMagenta = slog.FgHiMagenta
	FgHiCyan    = slog.FgHiCyan
	FgHiWhite   = slog.FgHiWhite
)

// Background text colors
const (
	BgBlack   = slog.BgBlack
	BgRed     = slog.BgRed
	BgGreen   = slog.BgGreen
	BgYellow  = slog.BgYellow
	BgBlue    = slog.BgBlue
	BgMagenta = slog.BgMagenta
	BgCyan    = slog.BgCyan
	BgWhite   = slog.BgWhite
)

// Background Hi-Intensity text colors
const (
	BgHiBlack   = slog.BgHiBlack
	BgHiRed     = slog.BgHiRed
	BgHiGreen   = slog.BgHiGreen
	BgHiYellow  = slog.BgHiYellow
	BgHiBlue    = slog.BgHiBlue
	BgHiMagenta = slog.BgHiMagenta
	BgHiCyan    = slog.BgHiCyan
	BgHiWhite   = slog.BgHiWhite
)

// ColoredValue wraps a value and renders it with ANSI colors when formatting.
type ColoredValue = slog.ColoredValue

// NewColoredValue creates a ColoredValue, it renders with colors when formatted.
// Deprecated: use slog.NewColoredValue.
func NewColoredValue(v any, color ...ColorAttribute) *ColoredValue {
	return slog.NewColoredValue(v, color...)
}

var (
	// CanColorStdout reports if we can color the Stdout.
	CanColorStdout = slog.CanColorStdout
	// CanColorStderr reports if we can color the Stderr.
	CanColorStderr = slog.CanColorStderr
)
