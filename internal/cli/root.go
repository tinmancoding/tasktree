package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/app"
	"github.com/tinmancoding/tasktree/internal/cache"
	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/gitx"
	"github.com/tinmancoding/tasktree/internal/metadata"
	"github.com/tinmancoding/tasktree/internal/registry"
	"github.com/tinmancoding/tasktree/internal/repoalias"
	tmplstore "github.com/tinmancoding/tasktree/internal/template"
)

type dependencies struct {
	git                  gitx.Client
	initService          app.InitService
	rootService          app.RootService
	listService          app.ListService
	listTasktreesService app.ListTasktreesService
	addService           app.AddGitService // backward-compat field name; same as addGitService
	addGitService        app.AddGitService
	addHTTPService       app.AddHTTPService
	addArchiveService    app.AddArchiveService
	addStaticService     app.AddStaticService
	addLocalService      app.AddLocalService
	applyService         app.ApplyService
	removeService        app.RemoveService
	statusService        app.StatusService
	pruneService         app.PruneService
	migrateService       app.MigrateService
	annotateService      app.AnnotateService
	aliasSet             app.RepoAliasSetService
	aliasRemove          app.RepoAliasRemoveService
	aliasList            app.RepoAliasListService
	aliasResolve         app.RepoAliasResolveService
	aliasRegister        app.RepoAliasRegisterDerivedService
	templateService      app.TemplateService
}

func defaultDependencies() dependencies {
	store := metadata.NewStore()
	git := gitx.NewClient()
	cacheRoot, err := cache.DefaultRoot()
	if err != nil {
		panic(err)
	}
	reg, err := registry.NewStore()
	if err != nil {
		panic(fmt.Sprintf("init registry store: %v", err))
	}
	repoAliasStore, err := repoalias.NewDefaultStore()
	if err != nil {
		panic(err)
	}

	// Template store: discover from current directory and user config.
	cwd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("get working directory: %v", err))
	}
	ts, err := tmplstore.NewStore(cwd)
	if err != nil {
		panic(fmt.Sprintf("init template store: %v", err))
	}

	addGitSvc := app.NewAddGitService(store, cache.NewManager(cacheRoot, git), git)
	return dependencies{
		git:                  git,
		initService:          app.NewInitServiceWithTemplates(store, reg, ts),
		rootService:          app.NewRootService(),
		listService:          app.NewListService(store),
		listTasktreesService: app.NewListTasktreesService(reg),
		addService:           addGitSvc,
		addGitService:        addGitSvc,
		addHTTPService:       app.NewAddHTTPService(store),
		addArchiveService:    app.NewAddArchiveService(store),
		addStaticService:     app.NewAddStaticService(store),
		addLocalService:      app.NewAddLocalService(store),
		applyService:         app.NewApplyService(store, cache.NewManager(cacheRoot, git), git),
		removeService:        app.NewRemoveService(store),
		statusService:        app.NewStatusService(store, git),
		pruneService:         app.NewPruneService(reg),
		migrateService:       app.NewMigrateService(store),
		annotateService:      app.NewAnnotateService(store),
		aliasSet:             app.NewRepoAliasSetService(repoAliasStore),
		aliasRemove:          app.NewRepoAliasRemoveService(repoAliasStore),
		aliasList:            app.NewRepoAliasListService(repoAliasStore),
		aliasResolve:         app.NewRepoAliasResolveService(repoAliasStore),
		aliasRegister:        app.NewRepoAliasRegisterDerivedService(repoAliasStore),
		templateService:      app.NewTemplateService(ts),
	}
}

func Execute() int {
	// Pre-parse -C/--chdir before full dependency initialization so that
	// cwd-sensitive setup (e.g. template store discovery) uses the right path.
	if err := preApplyChdir(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	cmd := NewRootCmd(defaultDependencies())
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// preApplyChdir scans args for the first -C / --chdir occurrence and calls
// os.Chdir so that subsequent cwd-sensitive initialization (e.g. template
// store discovery) uses the intended path.
func preApplyChdir(args []string) error {
	for i, arg := range args {
		var dir string
		switch {
		case (arg == "-C" || arg == "--chdir") && i+1 < len(args):
			dir = args[i+1]
		case strings.HasPrefix(arg, "-C="):
			dir = strings.TrimPrefix(arg, "-C=")
		case strings.HasPrefix(arg, "--chdir="):
			dir = strings.TrimPrefix(arg, "--chdir=")
		}
		if dir != "" {
			if err := os.Chdir(dir); err != nil {
				return fmt.Errorf("chdir %q: %w", dir, err)
			}
			return nil
		}
	}
	return nil
}

func NewRootCmd(deps dependencies) *cobra.Command {
	var verbose bool
	var chdir string
	deps.git = deps.git.WithDefaults()

	cmd := &cobra.Command{
		Use:           "tasktree",
		Short:         "Manage task-focused multi-repo workspaces",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if chdir != "" {
				if err := os.Chdir(chdir); err != nil {
					return fmt.Errorf("chdir %q: %w", chdir, err)
				}
			}
			if verbose {
				deps.git.SetVerboseWriter(cmd.ErrOrStderr())
			} else {
				deps.git.SetVerboseWriter(nil)
			}
			return nil
		},
	}
	cmd.SetErrPrefix("Error: ")
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Print git commands to stderr")
	cmd.PersistentFlags().StringVarP(&chdir, "chdir", "C", "", "Run as if started in `path` instead of the current working directory")
	cmd.AddCommand(
		newInitCmd(deps),
		newAddCmd(deps),
		newApplyCmd(deps),
		newRemoveCmd(deps),
		newRootSubcommand(deps),
		newListCmd(deps),
		newReposCmd(deps),
		newStatusCmd(deps),
		newPruneCmd(deps),
		newRepoCmd(deps),
		newMigrateCmd(deps),
		newAnnotateCmd(deps),
		newTemplateCmd(deps),
	)
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

	var legacyMetadata domain.LegacyMetadataError
	if errors.As(err, &legacyMetadata) {
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

	var repoAliasNotFound domain.RepoAliasNotFoundError
	if errors.As(err, &repoAliasNotFound) {
		return err
	}

	var repoAliasInUse domain.RepoAliasInUseError
	if errors.As(err, &repoAliasInUse) {
		return err
	}

	var invalidAnnotationKey domain.InvalidAnnotationKeyError
	if errors.As(err, &invalidAnnotationKey) {
		return err
	}

	var duplicateSourceName domain.DuplicateSourceNameError
	if errors.As(err, &duplicateSourceName) {
		return err
	}

	var invalidSourceName domain.InvalidSourceNameError
	if errors.As(err, &invalidSourceName) {
		return err
	}

	var invalidHTTPS domain.InvalidHTTPSSchemeError
	if errors.As(err, &invalidHTTPS) {
		return err
	}

	var sha256Mismatch domain.SHA256MismatchError
	if errors.As(err, &sha256Mismatch) {
		return err
	}

	var unknownArchiveFormat domain.UnknownArchiveFormatError
	if errors.As(err, &unknownArchiveFormat) {
		return err
	}

	var localSourceNotFound domain.LocalSourceNotFoundError
	if errors.As(err, &localSourceNotFound) {
		return err
	}

	// Template-related errors.
	var templateNotFound domain.TemplateNotFoundError
	if errors.As(err, &templateNotFound) {
		return err
	}

	var missingVariable domain.MissingVariableError
	if errors.As(err, &missingVariable) {
		return err
	}

	var unknownVariable domain.UnknownVariableError
	if errors.As(err, &unknownVariable) {
		return err
	}

	var invalidVariableName domain.InvalidVariableNameError
	if errors.As(err, &invalidVariableName) {
		return err
	}

	return fmt.Errorf("%w", err)
}
