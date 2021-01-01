package alloc

import (
	"strconv"
	"testing"
)

func TestNewKeys(t *testing.T) {
	var min uint32 = 1
	var max uint32 = 10

	keys := NewKeys(min, max)

	for n := 0; n < 2; n++ {
		for i := min; i <= max; i++ {
			id, err := keys.Acquire(strconv.FormatUint(uint64(i), 10))
			if err != nil || i != id {
				t.Fatalf("expect %d, but get %d, %v", i, id, err)
			}
		}
	}

	if _, err := keys.Acquire("exceed"); err != ErrNoFreeID {
		t.Fatalf("expect ErrNoFreeID, but get %v", err)
	}

	for i := max; i >= min; i-- {
		keys.Release(strconv.FormatUint(uint64(i), 10))
		id, err := keys.Acquire(strconv.FormatUint(uint64(i), 10))
		if err != nil || i != id {
			t.Fatalf("expect %d, but get %d, %v", i, id, err)
		}
	}
}
