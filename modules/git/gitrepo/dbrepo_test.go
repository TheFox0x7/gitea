// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepoHashedRelativePath(t *testing.T) {
	// sha256("1") = 6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b
	assert.Equal(t, "@hashed/6b/86/6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b.git", RepoCodeGitRepoRelativePathByID(1))
	assert.Equal(t, "@hashed/6b/86/6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b.wiki.git", RepoWikiGitRepoRelativePathByID(1))
	assert.True(t, IsHashedRepositoryLocation(RepoCodeGitRepoRelativePathByID(1)))
	assert.True(t, IsHashedRepositoryLocation(RepoWikiGitRepoRelativePathByID(2)))
	assert.False(t, IsHashedRepositoryLocation(RepoCodeGitRepoRelativePath("user2", "repo1")))
}
