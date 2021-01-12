package alloc

import (
	"testing"
)

func TestNewID(t *testing.T) {
	var min uint32 = 1
	var max uint32 = 10

	allocator := NewID(min, max)

	for i := min; i <= max; i++ {
		id, err := allocator.Acquire()
		if err != nil || i != id {
			t.Fatalf("expect %d, but get %d, %v", i, id, err)
		}
	}

	if _, err := allocator.Acquire(); err != ErrNoFreeID {
		t.Fatalf("expect ErrNoFreeID, but get %v", err)
	}

	for i := max; i >= min; i-- {
		allocator.Release(i)
		id, err := allocator.Acquire()
		if err != nil || i != id {
			t.Fatalf("expect %d, but get %d, %v", i, id, err)
		}
	}
}
