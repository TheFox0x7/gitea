// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package process

import (
	"gitea.dev/modules/cgroup"
)

// cgroupLogWarn logs cgroup setup problems. It is a variable to avoid an import cycle with the logging package.
var CgroupLogWarn = func(format string, args ...any) {}

// prepareCgroup makes the command start directly inside its cgroup bucket when the manager
// supports it (avoids the race between process start and cgroup assignment).
// It returns a closer for the cgroup directory fd which must be called after the command has started.
// All errors are logged and swallowed: a misconfigured cgroup setup must never break command execution.
func prepareCgroup(c *Cmd) (closer func(), err error) {
	closer = func() {}
	manager := cgroup.GetManager()
	if c.cgroupKey == "" || !manager.Ready() {
		return closer, nil
	}
	if manager.SupportsCloneIntoCgroup() {
		_, dir, errClone := manager.CloneIntoCgroup(c.Cmd, c.cgroupKey)
		if errClone != nil {
			CgroupLogWarn("Failed to prepare cgroup for command %q: %v, the command will run without cgroup limits", c.Cmd.Path, errClone)
			return closer, nil
		}
		return func() { _ = dir.Close() }, nil
	}
	return closer, nil
}

// addToCgroup assigns an already-started command to its cgroup bucket as a fallback
// when the manager cannot start it in the cgroup directly.
func addToCgroup(c *Cmd) error {
	manager := cgroup.GetManager()
	if c.cgroupKey == "" || !manager.Ready() || manager.SupportsCloneIntoCgroup() {
		return nil
	}
	if _, err := manager.AddCommand(c.Cmd, c.cgroupKey); err != nil {
		CgroupLogWarn("Failed to add command %q to its cgroup, it will run without cgroup limits: %v", c.Cmd.Path, err)
	}
	return nil
}
