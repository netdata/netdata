//go:build unix

package posix

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Endpoint lock files stay after a server closes, by design
// (docs/level1-posix-uds.md, "Endpoint lock"). Test service names are
// unique per run, so without this every run would leave new locks behind:
// the package removes the locks it named once all its tests have finished.
var testLocks struct {
	sync.Mutex
	paths []string
}

func trackTestLock(dir, service string) {
	testLocks.Lock()
	testLocks.paths = append(testLocks.paths, filepath.Join(dir, service+".lock"))
	testLocks.Unlock()
}

func TestMain(m *testing.M) {
	code := m.Run()
	testLocks.Lock()
	for _, p := range testLocks.paths {
		_ = os.Remove(p)
	}
	testLocks.Unlock()
	os.Exit(code)
}
