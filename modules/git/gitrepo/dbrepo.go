// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"gitea.dev/modules/util"
)

// HashedStoragePrefix is the top-level directory for the hashed repository layout (like Gitaly's "@hashed" storage).
const HashedStoragePrefix = "@hashed"

// RepoCodeGitRepoRelativePath returns the relative path for the code repository in the legacy name-based layout.
func RepoCodeGitRepoRelativePath(ownerName, repoName string) string {
	return util.PathJoinRelX(strings.ToLower(ownerName), strings.ToLower(repoName)+".git")
}

// RepoWikiGitRepoRelativePath returns the relative path for the wiki repository in the legacy name-based layout.
func RepoWikiGitRepoRelativePath(ownerName, repoName string) string {
	return util.PathJoinRelX(strings.ToLower(ownerName), strings.ToLower(repoName)+".wiki.git")
}

// RepoHashedRelativePath returns the relative path of a repository in the hashed layout:
// "@hashed/<hash[0:2]>/<hash[2:4]>/<hash><suffix>", where hash is the hex SHA256 of the repository ID,
// compatible with the layout used by Gitaly (GitLab).
func RepoHashedRelativePath(repoID int64, suffix string) string {
	hash := sha256.Sum256([]byte(strconv.FormatInt(repoID, 10)))
	hashHex := hex.EncodeToString(hash[:])
	return util.PathJoinRelX(HashedStoragePrefix, hashHex[:2], hashHex[2:4], hashHex+suffix)
}

// RepoCodeGitRepoRelativePathByID returns the relative path for the code repository in the hashed layout.
func RepoCodeGitRepoRelativePathByID(repoID int64) string {
	return RepoHashedRelativePath(repoID, ".git")
}

// RepoWikiGitRepoRelativePathByID returns the relative path for the wiki repository in the hashed layout.
func RepoWikiGitRepoRelativePathByID(repoID int64) string {
	return RepoHashedRelativePath(repoID, ".wiki.git")
}

// IsHashedRepositoryLocation returns true if the repository location belongs to the hashed layout.
func IsHashedRepositoryLocation(location string) bool {
	return strings.HasPrefix(location, HashedStoragePrefix+"/")
}

// CodeRepoByName returns an unmanaged repository facade for the code repository of the given owner and repository name.
// Usually it is used for migration fixes or repository adoption/creation/rename.
func CodeRepoByName(ownerName, repoName string) RepositoryFacade {
	return RepositoryUnmanaged(RepoCodeGitRepoRelativePath(ownerName, repoName))
}

// WikiRepoByName returns an unmanaged repository facade for the wiki repository of the given owner and repository name.
// Usually it is used for migration fixes or repository adoption/creation/rename.
func WikiRepoByName(ownerName, repoName string) RepositoryFacade {
	return RepositoryUnmanaged(RepoWikiGitRepoRelativePath(ownerName, repoName))
}
