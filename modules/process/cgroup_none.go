// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build !linux

package process

// CgroupLogWarn logs cgroup setup problems. It is a variable to avoid an import cycle with the logging package.
var CgroupLogWarn = func(format string, args ...any) {}

func prepareCgroup(c *Cmd) (closer func(), err error) { return func() {}, nil }
func addToCgroup(*Cmd) error                          { return nil }
