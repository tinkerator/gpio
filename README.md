# gpio - a package for using GPIOs under Linux

## Overview

The `gpio` package provides a native Go interface to the Linux GPIO
subsystem. The kernel API that the package uses is the _modern_ V2
character device one.

The API provided by this package centers around Get/Set methods for
GPIO `*Bank`s, Boolean `*Flag` arrays and numerical `*Vector`s.

Automated package documentation for this Go package should be
available from [![Go
Reference](https://pkg.go.dev/badge/zappem.net/pub/io/gpio.svg)](https://pkg.go.dev/zappem.net/pub/io/gpio).

## Getting started

Cross compiling to make a Raspberry Pi binary can be done as follows:
```
$ GOARCH=arm GOOS=linux go build tools/gpioutil.go
```

On a Raspberry Pi, running the compiled binary, `gpioutil`, on a
system with only one pair of pins connected (connect pin<18> and
pin<21>) will generate the indicated output:

```
pi@mypi:~ $ ./gpioutil --gpios=/dev/gpiochip0:18,19,20:21,22,23 --trace --pattern
2023/01/29 21:15:13 preparing <18>"GPIO18"[""](unused,active-high,input) for use as input
2023/01/29 21:15:13 preparing <19>"GPIO19"[""](unused,active-high,input) for use as input
2023/01/29 21:15:13 preparing <20>"GPIO20"[""](unused,active-high,input) for use as input
2023/01/29 21:15:13 preparing <21>"GPIO21"[""](unused,active-high,output) for use as output
2023/01/29 21:15:13 preparing <22>"GPIO22"[""](unused,active-high,output) for use as output
2023/01/29 21:15:13 preparing <23>"GPIO23"[""](unused,active-high,output) for use as output
2023/01/29 21:15:13 With GPIO tracing:
2023/01/29 21:15:13 000000000000000000000000
2023/01/29 21:15:13 000000000000000000000000
2023/01/29 21:15:13 000000000000000000000000
2023/01/29 21:15:13 000000000000000000000000
2023/01/29 21:15:13 000000000000000000000000
2023/01/29 21:15:13 001000000000000000000000
2023/01/29 21:15:13 001001000000000000000000
2023/01/29 21:15:13 011001000000000000000000
2023/01/29 21:15:14 111001000000000000000000
2023/01/29 21:15:14 110001000000000000000000
2023/01/29 21:15:14 110000000000000000000000
2023/01/29 21:15:15 100000000000000000000000
2023/01/29 21:15:15 000000000000000000000000
```

Here, the optional `--trace` argument logs the GPIO values
(`<0>"ID_SDA"` is the right most 0) in this output. Note, `<18>`
transitions to `1` after `<21>` is raised to `1` because of the wired
connection. Similarly, `<18>` lowers after `<21>` is lowered. Without
that wired connection, `<18>` is unchanged.

The optional argument `--vcd` can be used to generate a VCD trace file
instead of the simple one shown in the log above. To do this, you use
the following command line:

```
pi@mypi:~ $ ./gpioutil --gpios=/dev/gpiochip0:18,19,20:21,22,23 --trace --pattern --vcd=dump.vcd
```

The generated `dump.vcd` file can be viewed with
[`twave`](https://github.com/tinkerator/twave) or
[GTKWave](https://gtkwave.sourceforge.net/). In the case of the
former, the output looks like this:
```
$ ./twave --file dump.vcd 
[] : [$version iotracer $end]
             gpioutil.rpi.GPIO18-+
             gpioutil.rpi.GPIO19-|-+
             gpioutil.rpi.GPIO20-|-|-+
             gpioutil.rpi.GPIO21-|-|-|-+
             gpioutil.rpi.GPIO22-|-|-|-|-+
             gpioutil.rpi.GPIO23-|-|-|-|-|-+
                                 | | | | | |
2023-01-29 21:17:17.000000000000 0 0 0 0 x x
2023-01-29 21:17:17.000529100000 0 0 0 0 0 x
2023-01-29 21:17:17.001610000000 0 0 0 0 0 0
2023-01-29 21:17:17.002169700000 0 0 0 1 0 0
2023-01-29 21:17:17.003204900000 1 0 0 1 0 0
2023-01-29 21:17:17.503313000000 1 0 0 1 1 0
2023-01-29 21:17:18.004228600000 1 0 0 1 1 1
2023-01-29 21:17:18.504328800000 1 0 0 0 1 1
2023-01-29 21:17:18.506733700000 0 0 0 0 1 1
2023-01-29 21:17:19.004579600000 0 0 0 0 0 1
```

For a full list of command line options, `./gpioutil --help`.

For debugging purposes, I've been using a `HCDC HD040 Ver. 1.0` RPi
_hat_ which has some helpful LEDs on it to show the state of the
GPIOs as well as alternate connectors.

## TODOs

We might consider implementing an alternate backend `gpio.OpenFile()`
based access model. One that mirrors the `ioctl` based functions with
the legacy GPIO `/sys/class/gpio` files. However, some experimentation
with that indicates it is noticeably slower than the more modern one.

## License info

The `gpio` package is distributed with the same BSD 3-clause license
as that used by [golang](https://golang.org/LICENSE) itself.

## Reporting bugs and feature requests

The package `gpio` has been developed purely out of self-interest and
a curiosity for debugging physical IO projects, primarily on the
Raspberry Pi. Should you find a bug or want to suggest a feature
addition, please use the [bug
tracker](https://github.com/tinkerator/gpio/issues).
