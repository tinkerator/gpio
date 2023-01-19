// Program gpioutil is a rudimentary GPIO manipulation tool to demonstrate the
// ioctl based GPIO support in the gpio package.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"zappem.net/pub/io/gpio"
)

var (
	gpios = flag.String("gpios", "", "colon separated <device>:<ins>:<outs>")
	trace = flag.Bool("trace", false, "trace all IO")
	poll  = flag.Duration("poll", 4*time.Millisecond, "poll interval for sampling inputs")
)

// watcher is a rudimentary tracer abstraction.
type watcher struct {
	mu  sync.Mutex
	fmt string
}

// Sample displays a sample of data.
func (w *watcher) Sample(mask, value uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	log.Printf(w.fmt, value)
}

// cycle performs an experiment on the user specified gpios.
func cycle(ctx context.Context) {
	part := strings.Split(*gpios, ":")
	if len(part) != 3 {
		log.Fatalf("usage: %s <gpio-device-path>:[comma separated in gpios]:[comma separated out gpios]", os.Args[0])
	}
	b, err := gpio.OpenBank(ctx, part[0], *poll)
	if err != nil {
		log.Fatalf("failed to open gpios %q: %v", part[0], err)
	}
	defer b.Close()

	w := &watcher{
		fmt: "%1b",
	}

	max := -1
	var ins []int
	for _, v := range strings.Split(part[1], ",") {
		x, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			log.Fatalf("--gpios=...%q is not an integer: %v", err)
		}
		g := int(x)
		li, err := b.LineInfo(g)
		if err != nil {
			log.Fatalf("failed to find GPIO[%d] for input: %v", g, err)
		}
		log.Printf("preparing %v for use as input", li)
		if max < g {
			max = g
		}
		ins = append(ins, g)
	}

	var outs []int
	for _, v := range strings.Split(part[2], ",") {
		x, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			log.Fatalf("--gpios=...%q is not an integer: %v", err)
		}
		g := int(x)
		li, err := b.LineInfo(g)
		if err != nil {
			log.Fatalf("failed to find GPIO[%d] for output: %v", g, err)
		}
		log.Printf("preparing %v for use as output", li)
		if max < g {
			max = g
		}
		outs = append(outs, g)
	}
	if max < 0 {
		log.Fatal("need to list some GPIOs")
	}
	w.fmt = fmt.Sprintf("%%0%db", max+1)
	if *trace {
		b.SetTracer(w)
		log.Print("With GPIO tracing:")
	}
	for _, g := range ins {
		if err := b.Enable(g, true); err != nil {
			log.Fatalf("failed to enable %d: %v", g, err)
		}
		if err := b.Output(g, false); err != nil {
			log.Fatalf("failed to set to input %d: %v", g, err)
		}
	}
	for _, g := range outs {
		if err := b.Enable(g, true); err != nil {
			log.Fatalf("failed to enable %d: %v", g, err)
		}
		if err := b.Output(g, true); err != nil {
			log.Fatalf("failed to set to output %d: %v", g, err)
		}
	}

	for _, on := range []bool{true, false} {
		for _, g := range outs {
			b.Set(g, on)
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *gpios != "" {
		cycle(ctx)
		return
	}

	for _, f := range []string{"/dev/gpiochip0", "/dev/gpiochip1"} {
		b, err := gpio.OpenBank(ctx, f, *poll)
		if err != nil {
			log.Fatalf("failed to open gpios %q: %v", f, err)
		}
		fmt.Printf("chipinfo[%q]: %v\n", f, b)
		for i := 0; i < b.Lines(); i++ {
			li, err := b.LineInfo(i)
			if err != nil {
				log.Fatalf("failed to get lineinfo for %q: %v", f, err)
			}
			fmt.Printf("lineinfo[%q,%d]: %v\n", f, i, li)
		}
		b.Close()
	}

}
