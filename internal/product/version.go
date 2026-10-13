package product

import (
	"runtime/debug"
	"strings"
)

// version is set at release time:
//
//	-ldflags "-X github.com/tinmancoding/tasktree/v2/internal/product.version=2.0.0"
var version string

// Version returns the release version. Without a linker-set version it falls
// back to the module version recorded by `go install`, then to the VCS
// revision of a local `go build`, and finally to "dev".
func Version() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}

func resolveVersion(linked string, info *debug.BuildInfo, ok bool) string {
	if v := strings.TrimPrefix(strings.TrimSpace(linked), "v"); v != "" {
		return v
	}
	if !ok || info == nil {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return strings.TrimPrefix(v, "v")
	}
	var revision string
	dirty := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if dirty {
		revision += "-dirty"
	}
	return "dev+" + revision
}
