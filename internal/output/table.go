package output

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// TaskRow is one task in the human-readable list output.
type TaskRow struct {
	Handle   string
	Status   string
	Title    string
	ID       string
	Parent   string
	Progress string
}

func WriteTaskTable(w io.Writer, rows []TaskRow) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "HANDLE\tSTATUS\tTITLE\tID\tPARENT\tPROGRESS"); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Handle, r.Status, r.Title, r.ID, r.Parent, r.Progress); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// RepoRow is one repository in the human-readable info output.
type RepoRow struct {
	Name, Mode, Branch, Head, State string
	Ahead, Behind, Unpushed         int
}

func WriteRepoTable(w io.Writer, rows []RepoRow) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tMODE\tBRANCH\tHEAD\tSTATE\tAHEAD\tBEHIND\tUNPUSHED"); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\n", r.Name, r.Mode, r.Branch, r.Head, r.State, r.Ahead, r.Behind, r.Unpushed); err != nil {
			return err
		}
	}
	return tw.Flush()
}
