package domain

const KindTemplate = "Template"

// TemplateSpec represents a workspace template file (kind: Template).
type TemplateSpec struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"` // must be "Template"
	Metadata   TemplateMetadata `yaml:"metadata"`
	Parameters []ParameterSpec  `yaml:"parameters,omitempty"`
	Template   TasktreeTemplate `yaml:"template"`
}

// TemplateMetadata contains template-level metadata.
type TemplateMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// ParameterSpec defines a single template parameter.
type ParameterSpec struct {
	Name        string `yaml:"name"`
	Required    bool   `yaml:"required,omitempty"`
	Default     string `yaml:"default,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// TasktreeTemplate is the template body that mirrors the TasktreeSpec structure
// but may contain {{variable}} references in string fields.
type TasktreeTemplate struct {
	Metadata SpecMetadata  `yaml:"metadata"`
	Spec     WorkspaceSpec `yaml:"spec"`
}
