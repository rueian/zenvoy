package xds

import (
	"context"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"sync/atomic"
)

type SnapshotGenerator interface {
	Start(ctx context.Context) (<-chan cache.Snapshot, error)
}

type PassiveSnapshotGenerator struct {
	once int64
	ver  int
	ch   chan cache.Snapshot
}

func (p *PassiveSnapshotGenerator) Start(ctx context.Context) (<-chan cache.Snapshot, error) {
	if atomic.CompareAndSwapInt64(&p.once, 0, 1) {
		p.ch = make(chan cache.Snapshot)
		go func(p *PassiveSnapshotGenerator) {
			<-ctx.Done()
			atomic.StoreInt64(&p.once, 2)
			close(p.ch)
		}(p)
	}
	return p.ch, nil
}

func (p *PassiveSnapshotGenerator) InjectSnapshot(snapshot cache.Snapshot) {
	if atomic.LoadInt64(&p.once) == 1 {
		p.ch <- snapshot
	}
}
