//go:build race

package raw

// The race detector makes sync.Pool drop items on purpose, so pooled
// scratch allocates again and the allocation tests cannot hold.
const raceEnabled = true
