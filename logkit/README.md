# logkit

Non-blocking log transport for TinyGo targets. Host-testable, no Pico dependencies.

- `RingWriter` — an `io.Writer` backed by a fixed buffer. `Write` never blocks and never
  allocates; a record that does not fit is dropped whole and counted (`Dropped()`).
- `Drainer` — moves bytes from a `RingWriter` to a possibly blocking writer in small chunks
  (default 64 B, one USB full-speed packet). While `Ready()` is false the ring is discarded.
  After drops it emits `logkit: dropped=<total bytes>` at the next line boundary.

```go
ring := logkit.NewRingWriter(make([]byte, 4096))
drainer := &logkit.Drainer{Src: ring, Dst: machine.Serial, Ready: machine.Serial.DTR}
go drainer.Run()
logger := slog.New(slog.NewTextHandler(ring, nil))
```

## Why

TinyGo 0.43's USB-CDC `Write` (`machine/usb/cdc/usbcdc.go`) checks the host line state once,
then loops on `runtime.Gosched()` until its 512-byte TX ring has room. The ring only drains when
the host issues IN tokens, so when DTR is asserted but nothing reads the port, every goroutine
that logs blocks indefinitely. This happens, for example, after a reader closes a port with
`-hupcl`, or while a reader is stopped. With `Drainer`, only the drain goroutine blocks. Writers
keep running, and the ring drops records once it fills.

## Tasks

```sh
pnpm nx run logkit:test
pnpm nx run logkit:lint
```
