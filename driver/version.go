package driver

// When building any packages that import version, pass the build/install cmd
// ldflags like so:
//   go build -ldflags "-X github.com/UpCloudLtd/upcloud-csi/driver.version=0.0.1"

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
