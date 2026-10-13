package cli_test

import (
	"strings"
	"testing"

	"github.com/tinmancoding/tasktree/v2/internal/cli"
)

func expectCode(t *testing.T, want int, wantMsg string, args ...string) {
	t.Helper()
	_, _, err := run(t, args...)
	if err == nil {
		t.Fatalf("%v succeeded; want exit %d", args, want)
	}
	if got := cli.Code(err); got != want {
		t.Fatalf("%v exit=%d want %d: %v", args, got, want, err)
	}
	if wantMsg != "" && !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("%v error %q missing %q", args, err, wantMsg)
	}
}
