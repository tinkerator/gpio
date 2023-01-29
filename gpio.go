// Package gpio implements Go native access to GPIOs under Linux via the
// ioctl() based V2 kernel ABIs.
//
// The original ABI for GPIO access was based on text manipulation of
// files under the directory "/sys/class/gpio", but the more modern
// one, used by this package, performs configuration and access via
// ioctl() system calls and char devices: for example,
// "/dev/gpiochip0".
//
// The V2 definitions for the ABI used by this package are in the
// [kernel gpio.h header file].
//
// [kernel gpio.h header file]: https://github.com/torvalds/linux/blob/master/include/uapi/linux/gpio.h
package gpio // import "zappem.net/pub/io/gpio"

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/bits"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// These V2 constants are defined in:
//
// https://github.com/torvalds/linux/blob/master/include/uapi/linux/gpio.h
const (
	abi                   = 0xb4
	cmdGetChipinfo        = 0x01
	cmdGetLineinfo        = 0x05
	cmdGetLineinfoWatch   = 0x06
	cmdGetLine            = 0x07
	cmdGetLineinfoUnwatch = 0x0c
	cmdLineSetConfig      = 0x0d
	cmdLineGetValues      = 0x0e
	cmdLineSetValues      = 0x0f
)

// These are the generic 32-bit shift values
const (
	shiftCmd    = 0
	shiftABI    = 8
	shiftLength = 16
	shiftDir    = 30
)

// Pack the ioctl argument parameter.
func ioFn(cmd uint8, length int) uintptr {
	dir := uintptr(3)
	if cmd == cmdGetChipinfo {
		dir = 2
	}
	return (dir << shiftDir) |
		(uintptr(length) << shiftLength) |
		(abi << shiftABI) |
		(uintptr(cmd) << shiftCmd)
}

const (
	maxNameSize    = 32
	linesMax       = 64
	lineNumAttrMax = 10
)

// chipInfo holds the kernel ABI object 'struct gpiochip_info'.
type chipInfo struct {
	Name, Label [maxNameSize]byte
	Lines       uint32
}

// #define GPIO_V2_GET_LINEINFO_IOCTL _IOWR(0xB4, 0x05, struct gpio_v2_line_info)
// #define GPIO_V2_GET_LINEINFO_WATCH_IOCTL _IOWR(0xB4, 0x06, struct gpio_v2_line_info)

// LineAttrID holds attributes for line properties.
type LineAttrID uint32

const (
	LineAttrIDFlags        LineAttrID = 1
	LineAttrIDOutputValues LineAttrID = 2
	LineAttrIDDebounce     LineAttrID = 3
)

// LineFlag holds a bitmap of line properties.
type LineFlag uint64

const (
	// LineFlagUsed indicates the line is not available for this request.
	LineFlagUsed LineFlag = 1 << iota

	// LineFlagActiveLow indicates the active state is physical low.
	LineFlagActiveLow

	// LineFlagInput indicates line is an input (it can only be read).
	LineFlagInput

	// LineFlagOutput indicates line is an output and can be
	// written (and read).
	LineFlagOutput

	// LineFlagEdgeRising indicates the line detects inactive to
	// active transitions.
	LineFlagEdgeRising

	// LineFlagEdgeFalling indicates the line detects active to
	// inactive transitions.
	LineFlagEdgeFalling

	// LineFlagOpenDrain indicates the line is an open drain
	// output.
	LineFlagOpenDrain

	// LineFlagOpenSource indicates the line is an open source
	// output.
	LineFlagOpenSource

	// LineFlagBiasPullUp indicates the line has a pull-up bias
	// enabled.
	LineFlagBiasPullUp

	// LineFlagBiasPullDown indicates the line has a pull-down
	// bias enabled.
	LineFlagBiasPullDown

	// LineFlagBiasDisabled indicates the line has no bias
	// enabled.
	LineFlagBiasDisabled

	// LineFlagEventClockRealtime indicates the line events
	// include Realtime timestamps.
	LineFlagEventClockRealtime
)

var flagOns = []string{
	"in-use",
	"active-low",
	"input",
	"output",
	"rising-edge",
	"falling-edge",
	"open-drain",
	"open-source",
	"pull-up",
	"pull-down",
	"bias-disabled",
	"realtime-clock",
}

var flagOffs = []string{
	"unused",
	"active-high",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
	"",
}

// String summarizes the content of a flag.
func (flag LineFlag) String() string {
	var fs []string
	for i := 0; i < len(flagOns); i++ {
		m := LineFlag(1 << i)
		tok := flagOns[i]
		if flag&m == 0 {
			tok = flagOffs[i]
		}
		if tok == "" {
			continue
		}
		fs = append(fs, tok)
	}
	return strings.Join(fs, ",")
}

// LineAttribute is a representation of gpio_v2_line_attribute.
type LineAttribute struct {
	// ID is the identifier for selecting the Union member.
	ID LineAttrID

	// Padding is unused (=0) at present.
	Padding uint32

	// Union may be Flags or Values (uint64), or DebouncePeriodUs
	// (uint32).
	Union [8]byte
}

// Flags extracts the flags value from the LineAttr.
func (la *LineAttribute) Flags() (LineFlag, error) {
	if la.ID != LineAttrIDFlags {
		return 0, fmt.Errorf("invalid flags ID (got=%d, want=%d)", la.ID, LineAttrIDFlags)
	}
	buf := bytes.NewReader(la.Union[:])
	var flags LineFlag
	if err := binary.Read(buf, localEndianness, &flags); err != nil {
		return 0, err
	}
	return flags, nil
}

// Flags sets the flags value of a LineAttr.
func (la *LineAttribute) SetFlags(flags LineFlag) error {
	la.ID = LineAttrIDFlags
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, localEndianness, flags); err != nil {
		return err
	}
	copy(la.Union[:], buf.Bytes())
	return nil
}

// Values extracts the values value from the LineAttr.
func (la *LineAttribute) Values() (uint64, error) {
	if la.ID != LineAttrIDOutputValues {
		return 0, fmt.Errorf("invalid values ID (got=%d, want=%d)", la.ID, LineAttrIDOutputValues)
	}
	buf := bytes.NewReader(la.Union[:])
	var values uint64
	if err := binary.Read(buf, localEndianness, &values); err != nil {
		return 0, err
	}
	return values, nil
}

// Flags sets the values value of a LineAttr.
func (la *LineAttribute) SetValues(values uint64) error {
	la.ID = LineAttrIDOutputValues
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, localEndianness, values); err != nil {
		return err
	}
	copy(la.Union[:], buf.Bytes())
	return nil
}

// DebouncePeriod extracts the debounce period from the LineAttr.
func (la *LineAttribute) DebouncePeriod() (time.Duration, error) {
	if la.ID != LineAttrIDDebounce {
		return 0, fmt.Errorf("invalid debounce ID (got=%d, want=%d)", la.ID, LineAttrIDDebounce)
	}
	buf := bytes.NewReader(la.Union[:])
	var us uint32
	if err := binary.Read(buf, localEndianness, &us); err != nil {
		return 0, err
	}
	return time.Microsecond * time.Duration(us), nil
}

// SetDebouncePeriod sets the debounce time period for a LineAttr.
func (la *LineAttribute) SetDebouncePeriod(d time.Duration) error {
	la.ID = LineAttrIDDebounce
	us := uint32(d / time.Microsecond)
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, localEndianness, us); err != nil {
		return err
	}
	copy(la.Union[:4], buf.Bytes())
	return nil
}

// String serializes a LineAttribute.
func (la LineAttribute) String() string {
	switch la.ID {
	case 0:
		return ""
	case LineAttrIDFlags:
		f, _ := la.Flags()
		return f.String()
	case LineAttrIDOutputValues:
		v, _ := la.Values()
		return fmt.Sprint(v)
	case LineAttrIDDebounce:
		t, _ := la.DebouncePeriod()
		return t.String()
	default:
		return fmt.Sprintf("%d:%x", la.ID, la.Union)
	}
}

// LineInfo is a representation of gpio_v2_line_info.
type LineInfo struct {
	Name, Consumer  [maxNameSize]byte
	Offset, NumAttr uint32
	Flags           LineFlag
	Attrs           [lineNumAttrMax]LineAttribute
	Padding         [4]uint32
}

// String serializes a *LineInfo value.
func (li *LineInfo) String() string {
	if li == nil {
		return "<nil>"
	}
	var as []string
	for i := uint32(0); i < li.NumAttr; i++ {
		as = append(as, li.Attrs[i].String())
	}
	return fmt.Sprintf("<%d>%q[%q](%v)%s", li.Offset, cStr(li.Name[:]), cStr(li.Consumer[:]), li.Flags, strings.Join(as, ","))
}

// Label returns the Name field in the form of a proper Go string.
func (li *LineInfo) Label() string {
	if li == nil {
		return "<nil>"
	}
	return cStr(li.Name[:])
}

// #define GPIO_V2_GET_LINE_IOCTL _IOWR(0xB4, 0x07, struct gpio_v2_line_request)
// #define GPIO_V2_LINE_SET_CONFIG_IOCTL _IOWR(0xB4, 0x0D, struct gpio_v2_line_config)

// LineConfigAttribute is a representation of gpio_v2_line_config_attribute.
type LineConfigAttribute struct {
	Attr LineAttribute
	Mask uint64
}

// LineConfig is a representation of gpio_v2_line_config.
type LineConfig struct {
	Flags    LineFlag
	NumAttrs uint32
	Padding  [5]uint32
	Attrs    [lineNumAttrMax]LineConfigAttribute
}

// LineRequest is a representation of gpio_v2_line_request.
type LineRequest struct {
	Offsets                   [linesMax]uint32
	Consumer                  [maxNameSize]byte
	Config                    LineConfig
	NumLines, EventBufferSize uint32
	Padding                   [5]uint32
	Fd                        int32
}

// #define GPIO_V2_LINE_GET_VALUES_IOCTL _IOWR(0xB4, 0x0E, struct gpio_v2_line_values)
// #define GPIO_V2_LINE_SET_VALUES_IOCTL _IOWR(0xB4, 0x0F, struct gpio_v2_line_values)

// LineValues holds the kernel ABI object 'struct gpio_v2_line_values'.
type LineValues struct {
	// Bits and Mask are packed bits that index the LineRequest
	// offsets.  Unless we map everything in this GPIO bank, the
	// Bits and Mask fields do not equal the full set of lines as
	// contiguous bits.
	Bits, Mask uint64
}

// Tracer holds an optional tracing interface for gpio transitions.
type Tracer interface {
	// Sample records a sample of masked data.
	Sample(mask, value uint64)
}

// Bank provides access to a bank of GPIOs. It contains a cached copy
// of all GPIO state and is updated asynchronously. We use the kernel
// to track input events and the output events are managed by the bank
// code.
type Bank struct {
	// f is the file over which we can configure this bank of
	// GPIOs and through which we can read input events.
	f *os.File
	// name holds the kernel name for this bank
	name string
	// label holds the label of this bank
	label string
	// lines holds the number of GPIO lines.
	lines int

	// tracer, if non-nul, is used to store data traces.
	tracer Tracer

	// mu protects all subsequent fields.
	mu sync.Mutex

	// outs and outsMask capture the most recently written values
	// of all outputs. The package updates outsWhen when any value
	// changes. If outsMask is non-zero outsF holds an open file
	// for updating the output values. Note, outs and outsMask are
	// unpacked to align with the native bit offsets for the GPIO
	// device.
	outs, outsMask uint64
	outsWhen       time.Time
	outsF          *os.File
	setCh          chan bool

	// ins and insMask capture the most recently read value of all
	// inputs since time, insWhen. If insMask is non-zero insF
	// holds an open file for obtaining a more recent input
	// snapshot. Note, ins and insMask are unpacked to align with
	// the native bit offsets for the GPIO device.
	ins, insMask uint64
	insWhen      time.Time
	pollMask     uint64
	insF         *os.File
}

// Lines indicates how many lines are known to the bank.
func (b *Bank) Lines() int {
	if b == nil {
		return 0
	}
	return b.lines
}

// Convert C-string style bytes into a string (without invoking cgo).
func cStr(dat []byte) string {
	return string(dat[:cLen(dat)])
}

// ioctl performs an IOCTL on a file to access GPIO settings.
func ioctl(f *os.File, cmd uint8, data []byte) error {
	sc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	param := ioFn(cmd, len(data))
	sc.Control(func(fd uintptr) {
		_, _, eno := syscall.Syscall(syscall.SYS_IOCTL, fd, param, uintptr(unsafe.Pointer(&data[0])))
		if eno != 0 {
			err = eno
		}
	})
	return err
}

// refreshInputLocked is called locked and refills the input bits via
// a kernel call.
func (b *Bank) refreshInputLocked() {
	if present := b.f != nil; !present || b.insF == nil {
		return
	}
	ans := LineValues{
		Mask: b.pollMask,
	}
	setter := new(bytes.Buffer)
	binary.Write(setter, localEndianness, ans)
	if err := ioctl(b.insF, cmdLineGetValues, setter.Bytes()); err != nil {
		return
	}
	when := time.Now()
	buf := bytes.NewReader(setter.Bytes())
	if err := binary.Read(buf, localEndianness, &ans); err != nil {
		return
	}
	var val uint64
	for m := uint64(1); m <= b.insMask; m <<= 1 {
		if m&b.insMask == 0 {
			continue
		}
		if ans.Bits&1 != 0 {
			val |= m
		}
		ans.Bits >>= 1
	}
	if val == b.ins {
		return
	}

	b.ins = val
	b.insWhen = when
	if m := b.insMask | b.outsMask; m != 0 && b.tracer != nil {
		b.tracer.Sample(m, b.ins|b.outs)
	}
}

// pollInput periodically samples the input values.
func (b *Bank) pollInput(ctx context.Context, poll time.Duration) {
	t := time.NewTicker(poll)
	defer t.Stop()

	for present := true; present; {
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}

		b.mu.Lock()
		b.refreshInputLocked()
		b.mu.Unlock()
	}
}

// OpenBank opens the GPIO device file and returns a bank pointer.
// The ABI used by this package for the opened bank is the v2 one.
func OpenBank(ctx context.Context, path string, poll time.Duration) (*Bank, error) {
	f, err := os.OpenFile(path, syscall.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}

	b := &Bank{f: f}
	var data [2*maxNameSize + 4]byte
	if err := ioctl(b.f, cmdGetChipinfo, data[:]); err != nil {
		b.Close()
		return nil, err
	}
	var ans chipInfo
	buf := bytes.NewReader(data[:])
	if err := binary.Read(buf, localEndianness, &ans); err != nil {
		b.Close()
		return nil, err
	}
	b.name = cStr(ans.Name[:])
	b.label = cStr(ans.Label[:])
	b.lines = int(ans.Lines)

	// Because all masks are zero, there are no IO values known.
	go b.pollInput(ctx, poll)

	return b, nil
}

// Close closes the GPIO bank.
func (b *Bank) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.outsF != nil {
		b.outsF.Close()
		b.outsF = nil
	}
	if b.insF != nil {
		b.insF.Close()
		b.insF = nil
	}
	err := b.f.Close()
	b.f = nil
	return err
}

// endienness of hosting machine.
var localEndianness binary.ByteOrder

// init figures out what endianness the local machine has.
func init() {
	buf := [2]byte{}
	*(*uint16)(unsafe.Pointer(&buf[0])) = uint16(0x1234)
	if buf[0] == 0x34 {
		localEndianness = binary.LittleEndian
	} else {
		localEndianness = binary.BigEndian
	}
}

// cLen returns the number of characters in a C-style string contained
// in a byte array. Since the kernel operates with these types of
// string, we need a mechanism to interpret them.
func cLen(d []byte) int {
	var i int
	for i < len(d) && d[i] != 0 {
		i++
	}
	return i
}

// String summarizes a bank in the form of a string.
func (b *Bank) String() string {
	if b == nil {
		return "nil"
	}
	if b.f == nil {
		return "closed"
	}
	return fmt.Sprintf("%q %q (%d)", b.name, b.label, b.lines)
}

// LineInfo returns the current configuration of the line, g.
func (b *Bank) LineInfo(g int) (*LineInfo, error) {
	d := make([]byte, 2*maxNameSize+4+4+8+lineNumAttrMax*(4+4+8)+4*4 /* =256 */)
	setter := new(bytes.Buffer)
	binary.Write(setter, localEndianness, uint32(g))
	copy(d[2*maxNameSize:2*maxNameSize+4], setter.Bytes())
	if err := ioctl(b.f, cmdGetLineinfo, d); err != nil {
		return nil, err
	}
	ans := &LineInfo{}
	buf := bytes.NewReader(d[:])
	if err := binary.Read(buf, localEndianness, ans); err != nil {
		return nil, err
	}
	return ans, nil
}

// valid confirms that a GPIO value is valid for this bank.
func (b *Bank) valid(g int) error {
	if g < 0 || g >= b.lines {
		return fmt.Errorf("%d is not in %q range [0,%d)", g, b.name, b.lines)
	}
	return nil
}

// unpackMask converts a bit pattern into an array of line index
// values.
func unpackMask(mask uint64) []uint32 {
	var up []uint32
	for i := uint32(0); mask != 0; i++ {
		if mask&1 != 0 {
			up = append(up, i)
		}
		mask = mask >> 1
	}
	return up
}

// configGPIOs enables GPIOs for output and input purposes. It returns
// an access file descriptor for the specific GPIOs.
func (b *Bank) configGPIOs(flags LineFlag, mask uint64) (int, error) {
	up := unpackMask(mask)
	n := uint32(len(up))
	lr := LineRequest{
		Config: LineConfig{
			Flags: flags,
		},
		NumLines: n,
	}
	copy(lr.Consumer[:5], []byte("ioctl"))
	copy(lr.Offsets[:n], up[:])
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, localEndianness, lr); err != nil {
		return -1, err
	}
	if err := ioctl(b.f, cmdGetLine, buf.Bytes()); err != nil {
		return -1, err
	}
	res := bytes.NewReader(buf.Bytes())
	if err := binary.Read(res, localEndianness, &lr); err != nil {
		return -1, err
	}
	if lr.Fd < 0 {
		return -1, fmt.Errorf("bad filedes [%d, %b]", flags, up)
	}
	return int(lr.Fd), nil
}

// setOutsLocked is called with the bank locked, and outputs, via
// outsF, the current output line values.
func (b *Bank) setOutsLocked() error {
	if b.outsF == nil {
		return nil
	}
	m := b.outsMask
	var bits LineValues
	for uBit, bit := uint64(1), uint64(1); m != 0; m, uBit = m>>1, uBit<<1 {
		if m&1 == 0 {
			continue
		}
		bits.Mask |= bit
		if uBit&b.outs != 0 {
			bits.Bits |= bit
		}
		bit = bit << 1
	}
	buf := &bytes.Buffer{}
	if err := binary.Write(buf, localEndianness, bits); err != nil {
		return err
	}
	if m := b.insMask | b.outsMask; b.tracer != nil {
		b.tracer.Sample(m, b.ins|b.outs)
	}
	return ioctl(b.outsF, cmdLineSetValues, buf.Bytes())
}

// enableRWLocked is called with the bank locked, closes the existing
// read/writers and opens access to both as indicated by the masks.
func (b *Bank) enableRWLocked() error {
	if b.insF != nil {
		b.insF.Close()
		b.insF = nil
	}
	if b.outsF != nil {
		b.outsF.Close()
		b.outsF = nil
	}
	if b.outsMask != 0 {
		f, err := b.configGPIOs(LineFlagOutput, b.outsMask)
		if err != nil {
			return fmt.Errorf("failed to enable %b for output: %v", b.outsMask, err)
		}
		b.outsF = os.NewFile(uintptr(f), "outs")
	}
	if b.insMask != 0 {
		f, err := b.configGPIOs(LineFlagInput, b.insMask)
		if err != nil {
			return fmt.Errorf("failed to enable %b for input: %v", b.insMask, err)
		}
		b.insF = os.NewFile(uintptr(f), "ins")
	}
	b.pollMask = (1 << bits.OnesCount64(b.insMask)) - 1
	return b.setOutsLocked()
}

// Enable enables a GPIO for use by the program. Unless the GPIO is
// already enabled, by default, this configures the GPIO, g, as an
// INPUT.
func (b *Bank) Enable(g int, on bool) error {
	if err := b.valid(g); err != nil {
		return err
	}
	bit := uint64(1) << g

	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.insMask | b.outsMask
	if m&bit != 0 {
		return nil // already enabled.
	}

	b.insMask |= bit
	return b.enableRWLocked()
}

// Output configures an enabled GPIO's IO direction.
func (b *Bank) Output(g int, output bool) error {
	if err := b.valid(g); err != nil {
		return err
	}
	bit := uint64(1) << g

	b.mu.Lock()
	defer b.mu.Unlock()
	if output && b.outsMask&bit != 0 {
		return nil // already an output
	} else if !output && b.insMask&bit != 0 {
		return nil // already an input
	}
	if output {
		b.outsMask |= bit
		b.insMask ^= bit
	} else {
		b.insMask |= bit
		b.outsMask ^= bit
	}
	return b.enableRWLocked()
}

// SetHold locks a GPIO for the purpose of setting it. The set value
// is provided via returned channel. Once the channel is closed, with
// or without a value being written, the GPIO is unlocked. This
// function permits the value to be held until it is changed. If you
// don't need that behavior, just use Set().
func (b *Bank) SetHold(g int) (chan<- bool, error) {
	if err := b.valid(g); err != nil {
		return nil, err
	}
	bit := uint64(1) << g

	b.mu.Lock()
	if b.outsMask&bit == 0 {
		b.mu.Unlock()
		return nil, fmt.Errorf("%d is not write-enabled in %q bank", g, b.name)
	}
	ch := make(chan bool) // non buffered to ensure race free locking behavior
	go func() {
		for {
			// enter loop locked
			if b.setCh == nil {
				b.setCh = ch
			}
			if b.setCh == ch {
				select {
				case on, ok := <-ch: // only read while locked.
					defer b.mu.Unlock()
					b.setCh = nil
					if ok {
						if isOn := b.outs&bit != 0; on == isOn {
							return // nothing to do
						}
						b.outs ^= bit
						b.setOutsLocked()
						// Block until channel closed.
						for ok {
							_, ok = <-ch
						}
					}
					return
				default:
				}
			}
			b.mu.Unlock()
			runtime.Gosched()
			b.mu.Lock()
		}
	}()
	return ch, nil
}

// Set sets an output GPIO value atomically.
func (b *Bank) Set(g int, on bool) error {
	ch, err := b.SetHold(g)
	if err != nil {
		return err
	}
	ch <- on
	// Getting here, because ch is unbuffered, the SetHold() code
	// is locked until the write is fully performed. So b.Get()s
	// only see the bit set.
	close(ch)
	return nil
}

// Get reads the current (cached) GPIO value for outputs and performs
// a GPIO read for inputs.
func (b *Bank) Get(g int) (bool, error) {
	if err := b.valid(g); err != nil {
		return false, err
	}
	bit := uint64(1) << g

	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.insMask | b.outsMask
	if m&bit == 0 {
		return false, fmt.Errorf("%d is not enabled in %q bank", g, b.name)
	}
	if bit&b.outsMask != 0 {
		return bit&b.outs != 0, nil
	}
	b.refreshInputLocked()
	return bit&b.ins != 0, nil
}

// SetTracer begins tracing IO with the supplied tracer.
func (b *Bank) SetTracer(tracer Tracer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tracer = tracer
	if m := b.insMask | b.outsMask; m != 0 && tracer != nil {
		tracer.Sample(m, b.ins|b.outs)
	}
}
