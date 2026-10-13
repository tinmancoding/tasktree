package variable

import "testing"

func TestResolutionPrecedenceAndInlineDefault(t *testing.T) {
	t.Setenv("TASKTREE_VAR_ENV_VALUE", "environment")
	refs := Parse("{{cli_value}} {{env_value}} {{parameter_value}} {{inline_value | default:inline}}")
	got := Resolve(refs, map[string]string{"cli_value": "cli"}, map[string]string{"cli_value": "wrong", "env_value": "wrong", "parameter_value": "parameter"})
	want := map[string]string{"cli_value": "cli", "env_value": "environment", "parameter_value": "parameter", "inline_value": "inline"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s=%q want %q", k, got[k], v)
		}
	}
	if rendered := Render("{{cli_value}}/{{inline_value | default:no}}", got); rendered != "cli/inline" {
		t.Fatal(rendered)
	}
}
func TestParseKVRejectsMalformed(t *testing.T) {
	if _, err := ParseKVArgs([]string{"nope"}); err == nil {
		t.Fatal("expected error")
	}
}
