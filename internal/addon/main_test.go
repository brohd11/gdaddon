package addon

import (
	"os"
	"testing"
)

// TestMain keeps installs from reading the user's real config.yml: the gitignore toggle is
// off unless a test turns it on (withGitignore).
func TestMain(m *testing.M) {
	gitignoreEnabled = func() bool { return false }
	os.Exit(m.Run())
}
