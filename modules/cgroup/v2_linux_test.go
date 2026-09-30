// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package cgroup

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const NL = "\n"

func setupTestCgroupDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

func TestManagerV2BucketPath(t *testing.T) {
	m := NewManagerV2(Config{MountPoint: "/sys/fs/cgroup", HierarchyRoot: "gitea", Count: 4})
	// same key always maps to the same bucket, within [0, Count)
	for range 100 {
		p := m.bucketPath("repo-1")
		assert.Contains(t, []string{"repos-0", "repos-1", "repos-2", "repos-3"}, filepath.Base(p))
	}
	assert.Equal(t, m.bucketPath("repo-1"), m.bucketPath("repo-1"))
}

func TestManagerV2SetupAndAddCommand(t *testing.T) {
	if !IsUnifiedHierarchy("/sys/fs/cgroup") {
		t.Skip("host doesn't use cgroup v2 unified hierarchy")
	}
	// use a delegated-style temp hierarchy: only possible when /sys/fs/cgroup is writable (root)
	dir := filepath.Join("/sys/fs/cgroup", "gitea-test-"+t.Name())
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Skipf("cannot create test cgroup (needs delegated or writable cgroup fs): %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(dir) })

	m := NewManagerV2(Config{MountPoint: "/sys/fs/cgroup", HierarchyRoot: filepath.Base(dir), Count: 2})
	require.NoError(t, m.Setup())

	cmd := exec.Command("sleep", "0")
	require.NoError(t, cmd.Start())

	path, err := m.AddCommand(cmd, "test-repo")
	require.NoError(t, err)
	require.DirExists(t, filepath.Join("/sys/fs/cgroup", path))

	pidData, err := os.ReadFile(filepath.Join("/sys/fs/cgroup", path, "cgroup.procs"))
	require.NoError(t, err)
	assert.Contains(t, string(pidData), "0\n") // the command exited immediately, reaper moved it out; bucket exists and is usable

	require.NoError(t, cmd.Wait())

	stats, err := m.Stats()
	require.NoError(t, err)
	assert.NotEmpty(t, stats)
}

func TestReadTotalMemory(t *testing.T) {
	dir := setupTestCgroupDir(t, map[string]string{"memory.total": "1048576" + NL})
	assert.Equal(t, uint64(1024*1024), readTotalMemory(filepath.Join(dir, "memory.total")))

	dir = setupTestCgroupDir(t, map[string]string{"meminfo": "MemTotal:        1000 kB" + NL})
	assert.Equal(t, uint64(1000*1024), readTotalMemory(filepath.Join(dir, "meminfo")))
}

func TestAutoBucketCount(t *testing.T) {
	dir := setupTestCgroupDir(t, map[string]string{"memory.total": "2097152000\n"}) // 2000 MiB
	// 2000MiB / 512MiB = 3 buckets
	assert.Equal(t, 3, AutoBucketCount(dir, 512*1024*1024, 50))
	// capped at MaxBuckets
	assert.Equal(t, 2, AutoBucketCount(dir, 512*1024*1024, int64(2)))
	// low memory still gets one bucket
	assert.Equal(t, 1, AutoBucketCount(dir, 4*1024*1024*1024, 50))
	// missing information falls back to host memory
	assert.Equal(t, 1, AutoBucketCount(t.TempDir(), 512*1024*1024*1024, 1))
}

func TestIsUnifiedHierarchy(t *testing.T) {
	dir := setupTestCgroupDir(t, map[string]string{"cgroup.controllers": "cpuset cpu io memory\n"})
	assert.True(t, IsUnifiedHierarchy(dir))
	assert.False(t, IsUnifiedHierarchy(t.TempDir()))
}
