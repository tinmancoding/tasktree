package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/output"
)

func newTemplateCmd(deps dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage workspace templates",
	}
	cmd.AddCommand(
		newTemplateListCmd(deps),
		newTemplateShowCmd(deps),
		newTemplateValidateCmd(deps),
	)
	return cmd
}

func newTemplateListCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available templates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := deps.templateService.List()
			if err != nil {
				return formatError(err)
			}

			rows := make([]output.TemplateRow, len(entries))
			for i, e := range entries {
				var paramNames []string
				for _, p := range e.Parameters {
					if p.Required {
						paramNames = append(paramNames, p.Name)
					}
				}
				rows[i] = output.TemplateRow{
					Name:        e.Name,
					Description: e.Description,
					Parameters:  strings.Join(paramNames, ", "),
				}
			}
			return output.WriteTemplateTable(cmd.OutOrStdout(), rows)
		},
	}
}

func newTemplateShowCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name|path>",
		Short: "Show template details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.templateService.Show(args[0])
			if err != nil {
				return formatError(err)
			}
			return output.WriteTemplateDetail(cmd.OutOrStdout(), result.Spec, result.Location)
		},
	}
}

func newTemplateValidateCmd(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <name|path>",
		Short: "Validate a template file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nameOrPath := args[0]
			if err := deps.templateService.Validate(nameOrPath); err != nil {
				return formatError(err)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Template %q is valid.\n", nameOrPath)
			return err
		},
	}
}

// templateErrorHints adds template-specific formatting for domain errors.
// These are returned as-is since they already have good messages.
func templateErrorHints(err error) error {
	var templateNotFound domain.TemplateNotFoundError
	if _, ok := err.(domain.TemplateNotFoundError); ok {
		_ = templateNotFound
		return err
	}
	var missingVar domain.MissingVariableError
	if _, ok := err.(domain.MissingVariableError); ok {
		_ = missingVar
		return err
	}
	var unknownVar domain.UnknownVariableError
	if _, ok := err.(domain.UnknownVariableError); ok {
		_ = unknownVar
		return err
	}
	var invalidVarName domain.InvalidVariableNameError
	if _, ok := err.(domain.InvalidVariableNameError); ok {
		_ = invalidVarName
		return err
	}
	return nil
}
