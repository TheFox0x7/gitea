// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import "time"

// Event carries the legacy printf-path log metadata. The message itself is
// formatted by the slog engine; PC allows the caller to pass an explicitly
// captured program counter, Caller to prefill the caller name.
type Event struct {
	Time time.Time

	Caller   string
	Filename string
	Line     int

	Level Level

	PC uintptr
}
