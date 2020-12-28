package proxy

import "sync"

type XDS interface {
	GetIntendedEndpoints(port uint32) []string
	OnUpdated(func())
}

func NewStore() *store {
	return &store{endpoints: make(map[uint32][]string)}
}

type store struct {
	endpoints map[uint32][]string
	mu        sync.RWMutex
	cb        []func()
}

func (x *store) SetIntendedEndpoints(port uint32, endpoints []string) {
	x.mu.Lock()
	x.endpoints[port] = endpoints
	x.mu.Unlock()
	for _, f := range x.cb {
		f()
	}
}

func (x *store) GetIntendedEndpoints(port uint32) []string {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.endpoints[port]
}

func (x *store) OnUpdated(f func()) {
	x.cb = append(x.cb, f)
}
