// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"gitea.dev/modules/slog"
)

// Stack skips back the provided number of frames and returns a stack trace
// with source code.
// Deprecated: use slog.Stack.
func Stack(skip int) string {
	return slog.Stack(skip)
}
