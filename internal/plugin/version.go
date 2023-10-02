package plugin

import (
	"fmt"
	"os"
)

// When building any packages that import version, pass the build/install cmd
// ldflags like so:
//   go build -ldflags "-X github.com/UpCloudLtd/upcloud-csi/internal/plugin.version=0.0.1"

// TODO look at cleaner way to set these :(.
var (
	gitTreeState = "not a git tree" //nolint: gochecknoglobals // set by build
	commit       string             //nolint: gochecknoglobals // set by build
	version      string             //nolint: gochecknoglobals // set by build
)

func GetVersion() string {
	return version
}

// GetCommit returns the current commit hash value, as inserted at build time.
func GetCommit() string {
	return commit
}

// GetTreeState returns the current state of git tree, either "clean" or
// "dirty".
func GetTreeState() string {
	return gitTreeState
}

func PrintVersion() {
	fmt.Fprintf(os.Stdout, "%s - %s (%s)\n", GetVersion(), GetCommit(), GetTreeState())
}
