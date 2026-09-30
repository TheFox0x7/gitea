// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cgroup

import (
	"io"
	"os/exec"
)

// Stats holds the current usage statistics of a managed cgroup, read from cgroupfs files.
type Stats struct {
	// Path is the path of the cgroup
	Path string
	// MemoryUsage is the current memory usage in bytes, from `memory.current`
	MemoryUsage uint64
	// MemoryLimit is the memory limit in bytes, from `memory.max`
	MemoryLimit uint64
	// OOMKills is the accumulated OOM kill count, from the `oom_kill` field of `memory.events`
	OOMKills uint64
}

// Manager manages a cgroup hierarchy for Git subprocesses. A no-op manager is used on unsupported
// platforms or when the feature is disabled or misconfigured, so all methods are safe to call always.
type Manager interface {
	// Ready returns true if this manager is configured and operational.
	Ready() bool
	// AddCommand adds a started command to a cgroup determined by key.
	AddCommand(cmd *exec.Cmd, cgroupKey string) (path string, err error)
	// SupportsCloneIntoCgroup returns whether the manager can start a command directly in a cgroup,
	// which avoids the race between process start and cgroup assignment.
	SupportsCloneIntoCgroup() bool
	// CloneIntoCgroup configures the command to be started directly in the cgroup determined by key.
	// The returned closer must be closed after the command has been started.
	CloneIntoCgroup(cmd *exec.Cmd, cgroupKey string) (path string, closer io.Closer, err error)
	// Stats returns the usage statistics of all managed cgroups.
	Stats() ([]Stats, error)
}

var globalManager Manager = &NoopManager{}

// LogError logs cgroup setup errors. It is a variable to avoid an import cycle with the logging package,
// and is replaced by the logging package at startup.
var LogError = func(format string, args ...any) {}

// SetManager sets the global manager. It should be called once during startup.
func SetManager(m Manager) {
	globalManager = m
}

// GetManager returns the global manager, which is a no-op manager until one is set.
func GetManager() Manager {
	return globalManager
}

// NoopManager is used when cgroup management is disabled, unsupported or misconfigured.
type NoopManager struct{}

func (*NoopManager) Ready() bool { return false }
func (*NoopManager) AddCommand(*exec.Cmd, string) (string, error) {
	return "", nil
}
func (*NoopManager) SupportsCloneIntoCgroup() bool { return false }
func (*NoopManager) CloneIntoCgroup(*exec.Cmd, string) (string, io.Closer, error) {
	return "", nil, nil
}
func (*NoopManager) Stats() ([]Stats, error) { return nil, nil }
