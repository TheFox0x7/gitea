// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

// GitCgroups is the configuration for cgroup-based resource management of Git subprocesses (Linux, cgroup v2 only).
// It fails gracefully: if the feature is enabled but the environment doesn't support it, Git commands run as usual.
var GitCgroups = struct {
	Enabled         bool
	MountPoint      string `ini:"MOUNTPOINT"`
	HierarchyRoot   string
	Count           int
	MaxBuckets      int64
	MemoryPerBucket int64  `ini:"MEMORY_PER_BUCKET"`
	ParentMemoryMax int64  `ini:"PARENT_MEMORY_MAX"`
	ParentCPUMax    string `ini:"PARENT_CPU_MAX"`
	BucketMemoryMax int64  `ini:"BUCKET_MEMORY_MAX"`
	BucketCPUMax    string `ini:"BUCKET_CPU_MAX"`
}{
	Enabled:         false,
	MountPoint:      "",
	HierarchyRoot:   "gitea",
	Count:           0, // 0: scale automatically with available memory
	MaxBuckets:      50,
	MemoryPerBucket: 512 * 1024 * 1024,
	ParentMemoryMax: 0,  // 0: no limit
	ParentCPUMax:    "", // empty: no limit
	BucketMemoryMax: 0,
	BucketCPUMax:    "",
}
