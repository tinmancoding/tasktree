package product

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(mod string, settings ...debug.BuildSetting) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: mod}, Settings: settings}
	}
	tests := []struct {
		name, linked string
		info         *debug.BuildInfo
		ok           bool
		want         string
	}{
		{"linker wins", "2.1.0", info("v9.9.9"), true, "2.1.0"},
		{"linker v prefix", "v2.1.0", nil, false, "2.1.0"},
		{"module version", "", info("v2.0.1"), true, "2.0.1"},
		{"vcs revision", "", info("(devel)", debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef"}), true, "dev+0123456789ab"},
		{"dirty", "", info("(devel)", debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef"}, debug.BuildSetting{Key: "vcs.modified", Value: "true"}), true, "dev+0123456789ab-dirty"},
		{"nothing", "", info("(devel)"), true, "dev"},
		{"no build info", "", nil, false, "dev"},
	}
	for _, tt := range tests {
		if got := resolveVersion(tt.linked, tt.info, tt.ok); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
