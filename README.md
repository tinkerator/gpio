# gpio - a package for using GPIOs under Linux

## Overview

The `gpio` package provides a native Go interface to the Linux GPIO
subsystem. The kernel API that the package uses is the _modern_ V2
character device one.

## Getting started

Cross compiling to make a Raspberry Pi binary can be done as follows:
```
$ GOARCH=arm GOOS=linux go build tools/gpioutil.go
```

On a Raspberry Pi, running the compiled binary, `gpioutil`, on a
system with only one pair of pins connected (connect pin<10> and
pin<17>) will generate the indicated output:

```
pi@mypi:~ $ ./gpioutil --gpios=/dev/gpiochip0:10,9,11:17,27,22 --trace
2023/01/17 00:54:38 preparing <10>"SPI_MOSI"[""](unused,active-high,input) for use as input
2023/01/17 00:54:38 preparing <9>"SPI_MISO"[""](unused,active-high,input) for use as input
2023/01/17 00:54:38 preparing <11>"SPI_SCLK"[""](unused,active-high,input) for use as input
2023/01/17 00:54:38 preparing <17>"GPIO17"[""](unused,active-high,output) for use as output
2023/01/17 00:54:38 preparing <27>"GPIO27"[""](unused,active-high,output) for use as output
2023/01/17 00:54:38 preparing <22>"GPIO22"[""](unused,active-high,output) for use as output
2023/01/17 00:54:38 With GPIO tracing:
2023/01/17 00:54:38 0000000000000000000000000000
2023/01/17 00:54:38 0000000000000000000000000000
2023/01/17 00:54:38 0000000000000000000000000000
2023/01/17 00:54:38 0000000000000000000000000000
2023/01/17 00:54:38 0000000000000000000000000000
2023/01/17 00:54:38 0000000000100000000000000000
2023/01/17 00:54:38 0000000000100000010000000000
2023/01/17 00:54:38 1000000000100000010000000000
2023/01/17 00:54:39 1000010000100000010000000000
2023/01/17 00:54:39 1000010000000000010000000000
2023/01/17 00:54:39 1000010000000000000000000000
2023/01/17 00:54:40 0000010000000000000000000000
2023/01/17 00:54:40 0000000000000000000000000000
```

Here, the optional `--trace` argument logs the GPIO values
(`<0>"ID_SDA"` is the right most 0) in this output. Note, `<10>`
transitions to `1` after `<17>` is raised to `1` because of the wired
connection. Similarly, `<10>` lowers after `<17>` is lowered. Without
that wired connection, `<10>` is unchanged.

For debugging purposes, I've been using a `HCDC HD040 Ver. 1.0` RPi
_hat_ which has some helpful LEDs on it to show the state of the
GPIOs as well as alternate connectors.

## TODOs

We might consider implementing an alternate backend `gpio.OpenFile()`
based access model. One that mirrors the `ioctl` based functions with
the legacy GPIO `/sys/class/gpio` files. However, some experimentation
with that indicates it is noticeably slower than the more modern one.

We're also thinking about extending the tracing functions in some way
to help IO application debugging.

## License info

The `gpio` package is distributed with the same BSD 3-clause license
as that used by [golang](https://golang.org/LICENSE) itself.

## Reporting bugs and feature requests

The package `gpio` has been developed purely out of self-interest and
a curiosity for debugging physical IO projects, primarily on the
Raspberry Pi. Should you find a bug or want to suggest a feature
addition, please use the [bug
tracker](https://github.com/tinkerator/gpio/issues).
