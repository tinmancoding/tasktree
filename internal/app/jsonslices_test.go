package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/record"
)

// D-3: the zero value of every JSON document encodes lists as [] and never null.
func TestZeroValueJSONHasNoNullLists(t *testing.T) {
	for name, v := range map[string]any{
		"SyncReport":     SyncReport{},
		"ResolveResult":  ResolveResult{},
		"DoctorResult":   DoctorResult{},
		"TasktreeInfo":   TasktreeInfo{},
		"DefinitionInfo": DefinitionInfo{},
		"TemplateDetail": TemplateDetail{},
		"Task":           record.Task{},
	} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(name, err)
		}
		// parent is the one field that is legitimately null.
		got := strings.Replace(string(b), `"parent":null`, "", 1)
		if strings.Contains(got, "null") {
			t.Errorf("%s encodes null: %s", name, b)
		}
	}
}
