package wemdigo

import "sync"

// concurrent map
type cmap struct {
	sync.RWMutex
	m map[string]*Link
}

func (cm cmap) get(key string) (*Link, bool) {
	cm.RLock()
	defer cm.RUnlock()
	l, bool := cm.m[key]
	return l, bool
}

func (cm cmap) set(key string, l *Link) {
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
	cm.RLock()
	defer cm.RUnlock()
	return len(cm.m) == 0
}

func (cm cmap) linkIds() []string {
	cm.RLock()
	ids := make([]string, len(cm.m))
	i := 0
	for id := range cm.m {
		ids[i] = id
		i++
	}
	cm.RUnlock()
	return ids
}
