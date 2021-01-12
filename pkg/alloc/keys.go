package alloc

import (
	"sync"
)

func NewKeys(min, max uint32) *Keys {
	return &Keys{
		id:   NewID(min, max),
		keys: make(map[string]uint32),
	}
}

type Keys struct {
	id   *ID
	mu   sync.Mutex
	keys map[string]uint32
}

func (k *Keys) Acquire(key string) (id uint32, err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if id, ok := k.keys[key]; ok {
		return id, nil
	}
	if id, err = k.id.Acquire(); err == nil {
		k.keys[key] = id
	}
	return
}

func (k *Keys) Release(key string) {
	k.mu.Lock()
	if id, ok := k.keys[key]; ok {
		k.id.Release(id)
		delete(k.keys, key)
	}
	k.mu.Unlock()
}
