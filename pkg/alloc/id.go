package alloc

import "errors"

func NewID(min, max uint32) *ID {
	return &ID{
		min:  min,
		max:  max,
		next: min,
		used: make(map[uint32]bool),
	}
}

type ID struct {
	min  uint32
	max  uint32
	next uint32
	used map[uint32]bool
}

func (i *ID) Acquire() (uint32, error) {
	for id := i.next; id <= i.max; id++ {
		if !i.used[id] {
			i.used[id] = true
			i.next = id + 1
			return id, nil
		}
	}
	return 0, errors.New("no free id")
}

func (i *ID) Release(id uint32) {
	delete(i.used, id)
	if id < i.next && id >= i.min {
		i.next = id
	}
}
