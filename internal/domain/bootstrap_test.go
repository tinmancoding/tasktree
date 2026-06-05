package domain

import (
	"errors"
	"testing"
)

func TestValidateBootstrap(t *testing.T) {
	tests := []struct {
		name    string
		steps   []BootstrapStep
		wantErr error
	}{
		{
			name:  "nil list",
			steps: nil,
		},
		{
			name:  "empty list",
			steps: []BootstrapStep{},
		},
		{
			name: "valid single",
			steps: []BootstrapStep{
				{Name: "deps", Run: "npm ci"},
			},
		},
		{
			name: "valid multiple",
			steps: []BootstrapStep{
				{Name: "deps", Run: "npm ci", Workdir: "api"},
				{Name: "config", Run: "./gen.sh", Env: map[string]string{"X": "1"}},
			},
		},
		{
			name: "missing name",
			steps: []BootstrapStep{
				{Run: "npm ci"},
			},
			wantErr: EmptyBootstrapFieldError{Index: 0, Field: "name"},
		},
		{
			name: "missing run",
			steps: []BootstrapStep{
				{Name: "deps"},
			},
			wantErr: EmptyBootstrapFieldError{Index: 0, Field: "run"},
		},
		{
			name: "missing run second step",
			steps: []BootstrapStep{
				{Name: "deps", Run: "npm ci"},
				{Name: "config"},
			},
			wantErr: EmptyBootstrapFieldError{Index: 1, Field: "run"},
		},
		{
			name: "duplicate name",
			steps: []BootstrapStep{
				{Name: "deps", Run: "npm ci"},
				{Name: "deps", Run: "go mod download"},
			},
			wantErr: DuplicateBootstrapNameError{Name: "deps"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBootstrap(tt.steps)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %v, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr.Error() {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			// type identity check
			switch tt.wantErr.(type) {
			case EmptyBootstrapFieldError:
				var e EmptyBootstrapFieldError
				if !errors.As(err, &e) {
					t.Fatalf("expected EmptyBootstrapFieldError, got %T", err)
				}
			case DuplicateBootstrapNameError:
				var e DuplicateBootstrapNameError
				if !errors.As(err, &e) {
					t.Fatalf("expected DuplicateBootstrapNameError, got %T", err)
				}
			}
		})
	}
}
