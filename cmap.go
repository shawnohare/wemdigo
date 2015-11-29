package wemdigo

import "sync"

// concurrent map
type cmap struct {
	sync.RWMutex
	m map[string]*link
}

func (cm cmap) get(key string) (*link, bool) {
	cm.RLock()
	l, bool := cm.m[key]
	cm.RUnlock()
	return l, bool
}

func (cm cmap) set(key string, l *link) {
	cm.Lock()
	cm.m[key] = l
	cm.Unlock()
}

func (cm cmap) delete(key string) {
	cm.Lock()
	delete(cm.m, key)
	cm.Unlock()
}

func (cm cmap) isEmpty() bool {
	cm.Lock()
	empty := len(cm.m) == 0
	cm.Unlock()
	return empty
}
