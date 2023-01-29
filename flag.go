package gpio

import (
	"fmt"
	"runtime"
	"sync"
)

// Flag is a 64 bit array of boolean flags initialized to all false.
type Flag struct {
	mu     sync.Mutex
	value  uint64
	setCh  chan bool
	tracer Tracer
}

// NewFlag returns a new bank of flags.
func NewFlag() *Flag {
	return &Flag{}
}

// Lines returns the number of flag values.
func (f *Flag) Lines() int {
	if f == nil {
		return 0
	}
	return 64
}

// valid confirms a flag index value is valid.
func (f *Flag) valid(index int) error {
	if index < 0 || index >= 63 {
		return fmt.Errorf("invalid flag index got=%d, want [0,64)", index)
	}
	return nil
}

// Get reads the current value of the indexed flag. The index value is
// defined with domain [0,64).
func (f *Flag) Get(index int) (bool, error) {
	if err := f.valid(index); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return (f.value>>index)&1 != 0, nil
}

// SetHold locks a flag's value until its value is written and the
// returned setter channel is closed.
func (f *Flag) SetHold(index int) (chan<- bool, error) {
	if err := f.valid(index); err != nil {
		return nil, err
	}
	bit := uint64(1) << index
	ch := make(chan bool) // non buffered to ensure race free locking behavior
	f.mu.Lock()
	go func() {
		for {
			// enter loop locked
			if f.setCh == nil {
				f.setCh = ch
			}
			if f.setCh == ch {
				select {
				case on, ok := <-ch: // only read while locked.
					defer f.mu.Unlock()
					f.setCh = nil
					if ok {
						if isOn := f.value&bit != 0; on == isOn {
							return // nothing to do
						}
						f.value ^= bit
						if f.tracer != nil {
							f.tracer.Sample(^uint64(0), f.value)
						}
						// Block until channel closed.
						for ok {
							_, ok = <-ch
						}
					}
					return
				default:
				}
			}
			f.mu.Unlock()
			runtime.Gosched()
			f.mu.Lock()
		}
	}()
	return ch, nil
}

// Set is a serialized version of SetHold() that simply blocks until
// the specified flag is set to the on value.
func (f *Flag) Set(index int, on bool) error {
	ch, err := f.SetHold(index)
	if err == nil {
		ch <- on
		close(ch)
	}
	return err
}

// SetTracer sets or clears (tracer = nil) the flag tracer function.
func (f *Flag) SetTracer(tracer Tracer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tracer = tracer
}
