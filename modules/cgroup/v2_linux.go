// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package cgroup

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Config is the configuration for the cgroup hierarchy.
type Config struct {
	// MountPoint is where the cgroup2 filesystem is mounted, usually /sys/fs/cgroup
	MountPoint string
	// HierarchyRoot is the parent directory under the mount point, e.g. "gitea"
	HierarchyRoot string
	// Count is the number of Git subprocess cgroups ("buckets")
	Count int
	// ParentMemoryMax limits the memory of all Git subprocesses collectively, 0 means no limit
	ParentMemoryMax int64
	// ParentCPUMax limits the CPU of all Git subprocesses collectively, e.g. "50000 100000", empty means no limit
	ParentCPUMax string
	// BucketMemoryMax limits the memory per bucket, 0 means no limit
	BucketMemoryMax int64
	// BucketCPUMax limits the CPU per bucket, e.g. "50000 100000", empty means no limit
	BucketCPUMax string
}

// ManagerV2 manages a cgroup v2 hierarchy:
//
//	<MountPoint>/<HierarchyRoot>       the parent, resource limits applied collectively
//	<MountPoint>/<HierarchyRoot>/git   the Git subprocesses, N buckets created lazily under it
type ManagerV2 struct {
	cfg    Config
	bucket string // path of the Git subprocesses cgroup relative to the mount point
	locks  sync.Map
}

// NewManagerV2 creates a cgroup v2 manager from the settings.
func NewManagerV2(cfg Config) *ManagerV2 {
	return &ManagerV2{
		cfg:    cfg,
		bucket: filepath.Join(cfg.HierarchyRoot, "git"),
	}
}

// Setup creates the hierarchy and applies the configured limits.
func (m *ManagerV2) Setup() error {
	if err := m.makeDir(m.cfg.HierarchyRoot); err != nil {
		return fmt.Errorf("create hierarchy root: %w", err)
	}
	if err := m.makeDir(m.bucket); err != nil {
		return fmt.Errorf("create git cgroup: %w", err)
	}
	// enable controllers in the subtree so they take effect on the buckets
	if err := m.writeFile(filepath.Join(m.cfg.HierarchyRoot, "cgroup.subtree_control"), "+memory +cpu"); err != nil {
		return fmt.Errorf("enable subtree controllers: %w", err)
	}
	if err := m.writeOptional(filepath.Join(m.cfg.HierarchyRoot, "memory.max"), m.cfg.ParentMemoryMax); err != nil {
		return fmt.Errorf("set parent memory.max: %w", err)
	}
	if err := m.writeOptionalStr(filepath.Join(m.cfg.HierarchyRoot, "cpu.max"), m.cfg.ParentCPUMax); err != nil {
		return fmt.Errorf("set parent cpu.max: %w", err)
	}
	return nil
}

func (m *ManagerV2) Ready() bool                   { return true }
func (m *ManagerV2) SupportsCloneIntoCgroup() bool { return true }

func (m *ManagerV2) bucketPath(key string) string {
	return filepath.Join(m.bucket, "repos-"+strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(key)))%uint64(m.cfg.Count), 10))
}

func (m *ManagerV2) makeBucket(path string) error {
	type bucketState struct {
		once sync.Once
		err  error
	}
	v, _ := m.locks.LoadOrStore(path, &bucketState{})
	st := v.(*bucketState)
	st.once.Do(func() {
		st.err = m.makeDir(path)
		if st.err == nil {
			st.err = m.writeOptional(filepath.Join(path, "memory.max"), m.cfg.BucketMemoryMax)
		}
		if st.err == nil {
			st.err = m.writeOptionalStr(filepath.Join(path, "cpu.max"), m.cfg.BucketCPUMax)
		}
		if st.err != nil {
			LogError("Failed to set up cgroup %q: %v", path, st.err)
		}
	})
	return st.err
}

func (m *ManagerV2) AddCommand(cmd *exec.Cmd, key string) (string, error) {
	if cmd.Process == nil {
		return "", errors.New("cannot add a command that has not been started")
	}
	path := m.bucketPath(key)
	if err := m.makeBucket(path); err != nil {
		return "", err
	}
	return path, m.writeFile(filepath.Join(path, "cgroup.procs"), strconv.Itoa(cmd.Process.Pid))
}

func (m *ManagerV2) CloneIntoCgroup(cmd *exec.Cmd, key string) (string, io.Closer, error) {
	path := m.bucketPath(key)
	if err := m.makeBucket(path); err != nil {
		return "", nil, err
	}
	dir, err := os.Open(filepath.Join(m.cfg.MountPoint, path))
	if err != nil {
		return "", nil, err
	}
	cmd.SysProcAttr.UseCgroupFD = true
	cmd.SysProcAttr.CgroupFD = int(dir.Fd())
	return path, dir, nil
}

func (m *ManagerV2) Stats() ([]Stats, error) {
	entries, err := os.ReadDir(filepath.Join(m.cfg.MountPoint, m.bucket))
	if err != nil {
		return nil, err
	}
	stats := make([]Stats, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "repos-") {
			continue
		}
		path := filepath.Join(m.bucket, entry.Name())
		cgroupStat := Stats{Path: path}
		cgroupStat.MemoryUsage = m.readUint64(filepath.Join(path, "memory.current"))
		if limit, err := strconv.ParseUint(m.readFile(filepath.Join(path, "memory.max")), 10, 64); err == nil {
			cgroupStat.MemoryLimit = limit
		}
		cgroupStat.OOMKills = m.readUint64From(filepath.Join(path, "memory.events"), "oom_kill")
		stats = append(stats, cgroupStat)
	}
	return stats, nil
}

func (m *ManagerV2) makeDir(path string) error {
	// MkdirAll would fail if a controller file name collides with a directory, so create each level
	// separately with Mkdir and ignore "already exists".
	if err := os.Mkdir(filepath.Join(m.cfg.MountPoint, path), 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}

func (m *ManagerV2) writeFile(path string, value any) error {
	return os.WriteFile(filepath.Join(m.cfg.MountPoint, path), []byte(fmt.Sprint(value)), 0o644)
}

func (m *ManagerV2) writeOptional(path string, value int64) error {
	if value <= 0 {
		return nil
	}
	return m.writeFile(path, value)
}

func (m *ManagerV2) writeOptionalStr(path, value string) error {
	if value == "" {
		return nil
	}
	return m.writeFile(path, value)
}

func (m *ManagerV2) readFile(path string) string {
	content, err := os.ReadFile(filepath.Join(m.cfg.MountPoint, path))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func (m *ManagerV2) readUint64(path string) uint64 {
	v, _ := strconv.ParseUint(m.readFile(path), 10, 64)
	return v
}

func (m *ManagerV2) readUint64From(path, field string) uint64 {
	for _, line := range strings.Split(m.readFile(path), "\n") {
		if name, value, ok := strings.Cut(line, " "); ok && name == field {
			v, _ := strconv.ParseUint(value, 10, 64)
			return v
		}
	}
	return 0
}

// DetectMountPoint returns the mount point of the cgroup v2 unified hierarchy, or "" if not found.
func DetectMountPoint() string {
	for _, mountPoint := range []string{"/sys/fs/cgroup", "/mnt/cgroup2"} {
		if IsUnifiedHierarchy(mountPoint) {
			return mountPoint
		}
	}
	return ""
}

// IsUnifiedHierarchy returns true if mountPoint hosts a cgroup v2 unified hierarchy.
func IsUnifiedHierarchy(mountPoint string) bool {
	// the "cgroup.controllers" file only exists on the cgroup v2 unified hierarchy
	_, err := os.Stat(filepath.Join(mountPoint, "cgroup.controllers"))
	return err == nil
}

// AutoBucketCount scales the bucket count with the memory available to this process,
// so low-memory hosts get fewer buckets.
func AutoBucketCount(mountPoint string, memoryPerBucket, maxBuckets int64) int {
	// prefer the memory available to this process (container-aware), fall back to the host total
	totalMemory := readTotalMemory(filepath.Join(mountPoint, "memory.total"))
	if totalMemory == 0 {
		totalMemory = readTotalMemory("/proc/meminfo")
	}
	if totalMemory == 0 {
		return 1
	}
	count := int(totalMemory / uint64(memoryPerBucket))
	if count < 1 {
		count = 1
	}
	if count > int(maxBuckets) {
		count = int(maxBuckets)
	}
	return count
}

func readTotalMemory(path string) uint64 {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return 0
	}
	if strings.HasSuffix(path, "meminfo") {
		// /proc/meminfo format: "MemTotal:        4065636 kB"
		if fields[0] == "MemTotal:" && len(fields) >= 2 {
			v, _ := strconv.ParseUint(fields[1], 10, 64)
			return v * 1024
		}
		return 0
	}
	// cgroupfs "memory.total" contains a single number
	v, _ := strconv.ParseUint(fields[0], 10, 64)
	return v
}
