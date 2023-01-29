package gpio

import (
	"fmt"
	"runtime"
	"sync"
)

// Flag is a 64 bit array of boolean flags initialized to all
// false. While all bits are accessible, the mask value for this array
// is expanded as references are made to its indices. This helps cut
// down the size of generated traces if only one or two bits are used.
type Flag struct {
	mu     sync.Mutex
	value  uint64
	mask   uint64
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
// defined with domain [0,64). If an unset value is referenced for the
// first time it will return false, but if a tracer is enabled the
// implicit expansion of the flag mask will cause a trace sample to be
// generated.
func (f *Flag) Get(index int) (bool, error) {
	if err := f.valid(index); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	bit := uint64(1) << index
	if f.mask&bit == 0 {
		f.mask |= bit
		if f.tracer != nil {
			f.tracer.Sample(f.mask, f.value)
		}
	}
	return f.value&bit != 0, nil
}

// SetHold locks a flag's value until its value is written and the
// returned setter channel is closed. Once the value is written, if a
// tracer is enabled, a sample will be recorded which will include a
// mask value that includes the affected bit.
func (f *Flag) SetHold(index int) (chan<- bool, error) {
	if err := f.valid(index); err != nil {
		return nil, err
	}
	ch := make(chan bool) // non buffered to ensure race free locking behavior
	f.mu.Lock()
	// Before returning we need to be blocking all changes to
	// indexed flag state.  This guarantees that any reads of the
	// indexed state are not going to change while the caller
	// knows the state is held.
	for {
		if f.setCh == nil {
			f.setCh = ch
			break
		}
		f.mu.Unlock()
		runtime.Gosched()
		f.mu.Lock()
	}
	go func() {
		defer f.mu.Unlock()
		bit := uint64(1) << index
		for {
			// enter loop locked
			select {
			case on, ok := <-ch: // only read while locked.
				if ok {
					oldMask := f.mask
					f.mask |= bit
					old := f.value
					if on != (old&bit != 0) {
						f.value ^= bit
					}
					if f.tracer != nil && (old != f.value || oldMask != f.mask) {
						f.tracer.Sample(f.mask, f.value)
					}
					// Block until channel closed.
					for ok {
						_, ok = <-ch
					}
				}
				f.setCh = nil
				return
			default:
				f.mu.Unlock()
				runtime.Gosched()
				f.mu.Lock()
			}
		}
	}()
	return ch, nil
}

// Set is a serialized version of SetHold() that blocks until
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
	if tracer != nil {
		tracer.Sample(f.mask, f.value)
	}
}
