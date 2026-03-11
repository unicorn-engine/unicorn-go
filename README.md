# Unicorn Engine for go

This repository holds the bindings of [unicorn-engine](https://github.com/unicorn-engine/unicorn) for the Go language. This repository is not self-contained and requires a system installation of Unicorn.

## Installation

```bash
go get -u github.com/unicorn-engine/unicorn-go
```

By default, the Go toolchain will link your Go program against a global installation of Unicorn. If you wish to use a custom build of Unicorn or a local installation, you can do so by using the following variables:

```bash
CGO_CFLAGS="-Ipath/to/unicorn/include" CGO_LDFLAGS="-Lpath/to/unicorn/build -lunicorn" go build ...
```

## Usage

A very basic usage example follows *(Does not handle most errors for brevity. Please see sample.go for a more hygenic example):*

```go
package main

import (
    "fmt"
    uc "github.com/unicorn-engine/unicorn-go"
)

func main() {
    mu, _ := uc.NewUnicorn(uc.ARCH_X86, uc.MODE_32)
    // mov eax, 1234
    code := []byte{184, 210, 4, 0, 0}
    mu.MemMap(0x1000, 0x1000)
    mu.MemWrite(0x1000, code)
    if err := mu.Start(0x1000, 0x1000+uint64(len(code))); err != nil {
        panic(err)
    }
    eax, _ := mu.RegRead(uc.X86_REG_EAX)
    fmt.Printf("EAX is now: %d\n", eax)
}
```

An example program exercising far more Unicorn functionality and error handling can be found in [examples/x86_64_shellcode_with_hooks/main.go](examples/x86_64_shellcode_with_hooks/main.go).

## License

This project is released under the [GPL license](COPYING).

## Contact

[Contact us](http://www.unicorn-engine.org/contact/) via mailing list, email or twitter for any questions.

Join [our group](https://t.me/+lnNl0fPpyCYzZmVh) for instant feedback.

## Contribute

If you want to contribute, please pick up something from our [Github issues](https://github.com/unicorn-engine/unicorn-go/issues).

Please send pull requests to our dev branch.
