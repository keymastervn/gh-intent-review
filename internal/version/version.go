package version

import (
	_ "embed"
	"strings"
)

//go:embed version
var versionFile string

// Current is the release version read from the `version` file at the repo root.
var Current = strings.TrimSpace(versionFile)
