package main

import (
	"errors"
	"sync"
)

// Per-project locks serialize deploy, rollback and restore so two operations
// cannot unpack over each other. TryLock fails with a clear message.
var siteLocks sync.Map // slug -> *sync.Mutex

func withSiteLock(slug string, fn func() error) error {
	v, _ := siteLocks.LoadOrStore(slug, &sync.Mutex{})
	m := v.(*sync.Mutex)
	if !m.TryLock() {
		return errors.New("this project is already being deployed or restored; wait for it to finish")
	}
	defer m.Unlock()
	return fn()
}
