package gpio

import (
	"fmt"
	"runtime"
	"sync"
)

// Vector is an array of int64 values. Access to this vector is via
// two methods Get() and SetHold(). This API mirrors that of the
// (gpio) Bank and Flag arrays.
type Vector struct {
	mu sync.Mutex

	val   []int64
	setCh chan int64
}

// NewVector allocates a vector containing count numerical values all
// initialized to zero.
func NewVector(count uint) *Vector {
	return &Vector{
		val: make([]int64, count),
	}
}

// Lines returns the number of Vector values.
func (v *Vector) Lines() int {
	if v == nil {
		return 0
	}
	return len(v.val)
}

// valid confirms that the index value is valid.
func (v *Vector) valid(index int) error {
	if v == nil {
		return fmt.Errorf("nil vector has no index %d", index)
	}
	if index < 0 || index >= len(v.val) {
		return fmt.Errorf("invalid Vector index %d", index)
	}
	return nil
}

// Get returns the value of the index vector component.
func (v *Vector) Get(index int) (int64, error) {
	if err := v.valid(index); err != nil {
		return 0, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.val[index], nil
}

// SetHold provides a locked update mechanism for vector components.
// It locks the indexed value and permits a write to be subsequently
// made over the returned channel. The indexed value will remain
// locked until the returned channel is closed by the caller.
func (v *Vector) SetHold(index int) (chan<- int64, error) {
	if err := v.valid(index); err != nil {
		return nil, err
	}
	ch := make(chan int64)
	v.mu.Lock()
	for {
		if v.setCh == nil {
			v.setCh = ch
			break
		}
		v.mu.Unlock()
		runtime.Gosched()
		v.mu.Lock()
	}
	go func() {
		defer v.mu.Unlock()
		for {
			// enter loop locked
			select {
			case num, ok := <-ch:
				if ok {
					v.val[index] = num
					// block until ch closed by caller.
					for ok {
						_, ok = <-ch
					}
				}
				v.setCh = nil
				return
			default:
				v.mu.Unlock()
				runtime.Gosched()
				v.mu.Lock()
			}
		}
	}()
	return ch, nil
}

// Set is a convenience wrapper for SetHold that sets a vector
// component value atomically.
func (v *Vector) Set(index int, value int64) error {
	ch, err := v.SetHold(index)
	if err != nil {
		return nil
	}
	ch <- value
	close(ch)
	return nil
}
