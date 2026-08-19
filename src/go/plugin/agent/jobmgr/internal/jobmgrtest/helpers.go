package jobmgrtest

import "sync"

func onceClose(channel chan struct{}) func() {
	var once sync.Once
	return func() {
		once.Do(func() { close(channel) })
	}
}
