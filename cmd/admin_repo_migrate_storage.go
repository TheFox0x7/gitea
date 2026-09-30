// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitrepo"
	"gitea.dev/modules/globallock"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"

	"github.com/urfave/cli/v3"
)

func newRepoMigrateStorageCommand() *cli.Command {
	return &cli.Command{
		Name:  "repo-migrate-storage",
		Usage: "Migrate repositories on disk to the hashed storage layout (like Gitaly)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Only show what would be migrated, without moving anything",
			},
		},
		Action: runRepoMigrateStorage,
	}
}

func runRepoMigrateStorage(ctx context.Context, c *cli.Command) error {
	if setting.Repository.Layout != setting.RepositoryLayoutHashed {
		return fmt.Errorf("the [repository] LAYOUT setting must be %q before running this command, current: %q",
			setting.RepositoryLayoutHashed, setting.Repository.Layout)
	}
	if err := initDB(ctx); err != nil {
		return err
	}
	dryRun := c.Bool("dry-run")

	migrated, failed := 0, 0
	if err := db.Iterate(ctx, nil, func(ctx context.Context, repo *repo_model.Repository) error {
		movedCode, movedWiki, err := migrateRepoStorage(ctx, repo, dryRun)
		if err != nil {
			log.Error("Failed to migrate repository %s (id %d) to hashed storage: %v", repo.FullName(), repo.ID, err)
			failed++
			return nil
		}
		if movedCode || movedWiki {
			migrated++
			log.Info("Migrated repository %s (id %d) to hashed storage", repo.FullName(), repo.ID)
		}
		return nil
	}); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d repository(ies) migrated successfully, %d failed", migrated, failed)
	}
	log.Info("Repository storage migration finished: %d migrated", migrated)
	return nil
}

func migrateRepoStorage(ctx context.Context, repo *repo_model.Repository, dryRun bool) (movedCode, movedWiki bool, err error) {
	releaser, err := globallock.Lock(ctx, fmt.Sprintf("repo_working_%d", repo.ID))
	if err != nil {
		return false, false, fmt.Errorf("lock repo %d: %w", repo.ID, err)
	}
	defer releaser()

	if err := repo.LoadOwner(ctx); err != nil {
		return false, false, err
	}

	if movedCode, err = migrateOneGitRepoStorage(ctx, repo,
		gitrepo.CodeRepoByName(repo.OwnerName, repo.Name),
		gitrepo.RepositoryUnmanaged(gitrepo.RepoCodeGitRepoRelativePathByID(repo.ID)),
		dryRun,
	); err != nil {
		return false, false, err
	}
	if movedWiki, err = migrateOneGitRepoStorage(ctx, repo,
		gitrepo.WikiRepoByName(repo.OwnerName, repo.Name),
		gitrepo.RepositoryUnmanaged(gitrepo.RepoWikiGitRepoRelativePathByID(repo.ID)),
		dryRun,
	); err != nil {
		return false, false, err
	}
	return movedCode, movedWiki, nil
}

func migrateOneGitRepoStorage(ctx context.Context, repo *repo_model.Repository, oldRepo, newRepo git.RepositoryFacade, dryRun bool) (bool, error) {
	oldExist, err := git.IsRepositoryExist(ctx, oldRepo)
	if err != nil {
		return false, err
	}
	if !oldExist {
		return false, nil
	}
	newExist, err := git.IsRepositoryExist(ctx, newRepo)
	if err != nil {
		return false, err
	}
	if newExist {
		return false, fmt.Errorf("target path %s for repository %s (id %d) already exists, please resolve it manually",
			newRepo.GitRepoLocation(), repo.FullName(), repo.ID)
	}
	if dryRun {
		log.Info("[dry-run] Would move %s to %s", oldRepo.GitRepoLocation(), newRepo.GitRepoLocation())
		return true, nil
	}
	if err := git.RenameRepository(ctx, oldRepo, newRepo); err != nil {
		return false, fmt.Errorf("move repository directory: %w", err)
	}
	return true, nil
}
