// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package git

import (
	"path/filepath"

	"gitea.dev/modules/cgroup"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
)

// setupGitCgroups initializes cgroup management for Git subprocesses from the [git.cgroups] settings.
// It fails gracefully: on any misconfiguration or unsupported environment (e.g. non-delegated
// containers, cgroup v1) it logs a warning and leaves the no-op manager in place,
// so Git commands always keep working unconfined.
func setupGitCgroups() {
	cfg := setting.GitCgroups
	if !cfg.Enabled {
		return
	}
	mountPoint := cfg.MountPoint
	if mountPoint == "" {
		mountPoint = cgroup.DetectMountPoint()
	}
	if mountPoint == "" {
		log.Warn("Cgroup support is enabled but no cgroup v2 filesystem could be detected, Git commands will run without cgroup limits")
		return
	}
	if !cgroup.IsUnifiedHierarchy(mountPoint) {
		log.Warn("Cgroup support requires the cgroup v2 unified hierarchy, Git commands will run without cgroup limits")
		return
	}
	count := cfg.Count
	if count <= 0 {
		count = cgroup.AutoBucketCount(mountPoint, cfg.MemoryPerBucket, cfg.MaxBuckets)
	}
	manager := cgroup.NewManagerV2(cgroup.Config{
		MountPoint:      mountPoint,
		HierarchyRoot:   cfg.HierarchyRoot,
		Count:           count,
		ParentMemoryMax: cfg.ParentMemoryMax,
		ParentCPUMax:    cfg.ParentCPUMax,
		BucketMemoryMax: cfg.BucketMemoryMax,
		BucketCPUMax:    cfg.BucketCPUMax,
	})
	if err := manager.Setup(); err != nil {
		log.Warn("Cgroup support is enabled but the hierarchy could not be set up (%v), Git commands will run without cgroup limits", err)
		return
	}
	cgroup.SetManager(manager)
	log.Info("Cgroup support is enabled: %d Git subprocess buckets under %q", count, filepath.Join(mountPoint, cfg.HierarchyRoot))
}
