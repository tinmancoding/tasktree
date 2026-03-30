package output

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/tinmancoding/tasktree/internal/domain"
)

// TasktreeRow is a single row in the tasktree list table.
// The CLI layer is responsible for mapping app.TasktreeListEntry into this type.
type TasktreeRow struct {
	Name   string
	Path   string
	Status string // empty string means OK
}

// WriteTasktreeTable renders the list of known tasktrees as a table.
// Rows with a non-empty Status are annotated in a STATUS column.
func WriteTasktreeTable(w io.Writer, rows []TasktreeRow) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No tasktrees registered.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tPATH\tSTATUS"); err != nil {
		return err
	}
	for _, r := range rows {
		status := r.Status
		if status != "" {
			status = "(" + status + ")"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.Path, status); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func WriteRepoTable(w io.Writer, repos []domain.RepoSpec) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tPATH\tREF\tBRANCH"); err != nil {
		return err
	}
	for _, repo := range repos {
		branch := repo.Branch
		if branch == "" {
			branch = "-"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", repo.Name, repo.Path, repo.Checkout, branch); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func WriteStatusTable(w io.Writer, tasktreeName, root string, repos []struct {
	Name  string
	Path  string
	Head  string
	State string
}) error {
	if _, err := fmt.Fprintf(w, "Tasktree: %s\n", tasktreeName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Root: %s\n\n", root); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "REPO\tPATH\tHEAD\tSTATE"); err != nil {
		return err
	}
	for _, repo := range repos {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", repo.Name, repo.Path, repo.Head, repo.State); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func WriteRepoAliasTable(w io.Writer, aliases []struct {
	Alias string
	URL   string
}) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ALIAS\tURL"); err != nil {
		return err
	}
	for _, alias := range aliases {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", alias.Alias, alias.URL); err != nil {
			return err
		}
	}
	return tw.Flush()
}
