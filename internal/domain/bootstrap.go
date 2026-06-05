package domain

// ValidateBootstrap checks the workspace bootstrap steps for structural
// validity: each step must have a non-empty name and run, and step names must
// be unique within the list. workdir is not resolved here (it requires the
// on-disk workspace root and is validated at apply time).
func ValidateBootstrap(steps []BootstrapStep) error {
	seen := make(map[string]struct{}, len(steps))
	for i, step := range steps {
		if step.Name == "" {
			return EmptyBootstrapFieldError{Index: i, Field: "name"}
		}
		if step.Run == "" {
			return EmptyBootstrapFieldError{Index: i, Field: "run"}
		}
		if _, dup := seen[step.Name]; dup {
			return DuplicateBootstrapNameError{Name: step.Name}
		}
		seen[step.Name] = struct{}{}
	}
	return nil
}
