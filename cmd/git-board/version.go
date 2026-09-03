package main

import "runtime/debug"

// buildVersion reads the module version Go embedded when this binary was
// built — "go install .../git-board@v0.3.0" stamps v0.3.0 automatically,
// so this never needs manual bumping (a hand-maintained version constant
// is exactly the kind of thing that goes stale, the same failure mode
// that made "go install ...@latest" silently serve v0.1.0 for a while).
// "(devel)" for a local, non-tagged build (e.g. "go build" from a clone).
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}
