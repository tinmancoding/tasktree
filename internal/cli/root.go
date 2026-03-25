package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

type dependencies struct {
	initService   app.InitService
	rootService   app.RootService
	listService   app.ListService
	addService    app.AddService
	removeService app.RemoveService
	statusService app.StatusService
}

func defaultDependencies() dependencies {
	store := metadata.NewStore()
	git := gitx.NewClient()
	cacheRoot, err := cache.DefaultRoot()
	if err != nil {
		panic(err)
	}
	return dependencies{
		initService:   app.NewInitService(store),
		rootService:   app.NewRootService(),
		listService:   app.NewListService(store),
		addService:    app.NewAddService(store, cache.NewManager(cacheRoot, git), git),
		removeService: app.NewRemoveService(store),
		statusService: app.NewStatusService(store, git),
	}
}

func Execute() int {
	cmd := NewRootCmd(defaultDependencies())
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func NewRootCmd(deps dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "tasktree",
		Short:         "Manage task-focused multi-repo workspaces",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetErrPrefix("Error: ")
	cmd.AddCommand(newInitCmd(deps), newAddCmd(deps), newRemoveCmd(deps), newRootSubcommand(deps), newListCmd(deps), newStatusCmd(deps))
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return formatError(err)
	})
	return cmd
}

func formatError(err error) error {
	var notInTasktree domain.NotInTasktreeError
	if errors.As(err, &notInTasktree) {
		return err
	}

	var metadataExists domain.MetadataExistsError
	if errors.As(err, &metadataExists) {
		return err
	}

	var duplicateRepo domain.DuplicateRepoNameError
	if errors.As(err, &duplicateRepo) {
		return err
	}

	var destinationExists domain.DestinationExistsError
	if errors.As(err, &destinationExists) {
		return err
	}

	var invalidRepoName domain.InvalidRepoNameError
	if errors.As(err, &invalidRepoName) {
		return err
	}

	var unresolvedRef domain.UnresolvedRefError
	if errors.As(err, &unresolvedRef) {
		return err
	}

	var branchExists domain.BranchExistsError
	if errors.As(err, &branchExists) {
		return err
	}

	var repoNotFound domain.RepoNotFoundError
	if errors.As(err, &repoNotFound) {
		return err
	}

	var unsafePath domain.UnsafePathError
	if errors.As(err, &unsafePath) {
		return err
	}

	var invalidBranchName domain.InvalidBranchNameError
	if errors.As(err, &invalidBranchName) {
		return err
	}

	return fmt.Errorf("%w", err)
}
