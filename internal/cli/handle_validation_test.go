package cli_test

import (
	"strings"
	"testing"
)

func TestM16HandlesUseOneRule(t *testing.T) {
	setupRecords(t)
	long := strings.Repeat("a", 65)
	expectCode(t, 2, "invalid handle", "new", "bad", "--handle", "has space")
	expectCode(t, 2, "invalid handle", "new", "bad", "--handle", long)
	expectCode(t, 2, "invalid handle", "new", "bad", "--jira", "ABC 1")
	expectCode(t, 2, "invalid handle", "new", "bad", "--jira", long)
	expectCode(t, 2, "invalid handle", "import", "/nonexistent", "--handle", "has space")
	// A 64-character handle and a 40+ character title still work.
	if _, _, err := run(t, "new", strings.Repeat("title ", 20), "--handle", strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, "new", strings.Repeat("title ", 20))
	if err != nil || len(strings.Fields(out)[0]) > 40 {
		t.Fatalf("slug %q %v", out, err)
	}
}
