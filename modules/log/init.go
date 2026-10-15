// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"gitea.dev/modules/process"
	"gitea.dev/modules/slog"
	"gitea.dev/modules/util/rotatingfilewriter"
)

func init() {
	rotatingfilewriter.ErrorPrintf = slog.FallbackErrorf

	process.TraceCallback = func(skip int, start bool, pid process.IDType, description string, parentPID process.IDType, typ string) {
		if start && parentPID != "" {
			Log(skip+1, TRACE, "Start %s: %s (from %s) (%s)", NewColoredValue(pid, FgHiYellow), description, NewColoredValue(parentPID, FgYellow), NewColoredValue(typ, Reset))
		} else if start {
			Log(skip+1, TRACE, "Start %s: %s (%s)", NewColoredValue(pid, FgHiYellow), description, NewColoredValue(typ, Reset))
		} else {
			Log(skip+1, TRACE, "Done %s: %s", NewColoredValue(pid, FgHiYellow), NewColoredValue(description, Reset))
		}
	}
}
