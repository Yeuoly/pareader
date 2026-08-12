# PA Reader Host

This Go module contains the PRHP host, terminal reader application, and Windows
AimeIO 1.0 DLL. It opens only the PRHP HID interface with Usage Page `0xFF50`;
Spice2x opens the separate CardIO interface itself.

## Runtime model

One process-level service supervises HID connections. `aime_io_init` starts
that service and succeeds even when no reader is connected. While disconnected,
the service enumerates a matching PRHP interface every 500 ms. Each successful
open creates one session with one blocking reader goroutine:

```text
open HID -> start ReadLoop -> verify PRHP version -> online
    ^                                              |
    +--- wait 500 ms <- ReadLoop returns error <---+
```

The router sends `RESPONSE` reports to the session's sequence caller and sends
`CARD_STATE` signals to its card handler. The handler decodes the signal and
atomically replaces the session's current card:

```go
router.Handle(protocol.OpcodeCardState, func(raw []byte) error {
    card, err := protocol.DecodeCardStateSignal(raw)
    if err != nil {
        return err
    }
    session.latest.Store(&card)
    return nil
})
```

The blocking ReadLoop is the connection lifetime. When it returns, the service
removes that session, exposes `NONE`, closes the old HID handle, and resumes
enumeration. There is no heartbeat or online-state polling.

There is no card-read request. `aime_io_nfc_poll` remains exported for ABI
compatibility but performs no transport operation. While disconnected, the
card getters return `S_FALSE`, which is the same ABI result as no card being
present. Reconnecting does not require another call from Segatools.

HID writes are copied into a bounded queue and owned by one writer goroutine.
Request-response commands such as `GET_VERSION` still use sequence allocation
and the time-based caller cache. One-way signals such as `SET_LED` bypass the
caller.

## Test and run

```sh
go test ./...
go run ./cmd/reader
```

The reader application defaults to VID `5041`, PID `5245`, Usage Page `FF50`,
and Usage `0001`:

```sh
go run ./cmd/reader -vid 5041 -pid 5245 -serial SERIAL
```

## Build the DLL

```sh
go install mvdan.cc/garble@v0.14.2
make dll
```

`make dll` targets Windows amd64 with cgo, applies Garble tiny mode and C link
time optimization, and needs a Windows cgo compiler such as MinGW-w64 when run
on another operating system. It emits `build/aimeio.dll` and the generated C
header. Tiny mode removes panic, fatal-error, trace, and source-position
diagnostics from the release DLL. HIDAPI remains the Windows HID backend, but
its internal C API is not re-exported from the AimeIO DLL.

Garble `v0.14.2` is pinned because it builds with the module's Go 1.24 toolchain.
`make dll-plain` produces a stripped DLL without Garble when runtime diagnostics
are required. Override `GARBLE` or `WINDOWS_CC` to select non-default tool paths.

| Environment variable | Default | Meaning |
|---|---|---|
| `PAREADER_VID` | `5041` | USB vendor ID in hexadecimal |
| `PAREADER_PID` | `5245` | USB product ID in hexadecimal |
| `PAREADER_SERIAL` | empty | Optional exact serial match |
| `PAREADER_TIMEOUT` | `1s` | Local request timeout for request-response commands |

## Error codes

Errors defined by PA Reader are named constants whose printable value is a
stable five-character code:

```go
const ErrInvalidReport errcode.Code = "E0401"
```

This keeps release diagnostics small while preserving normal Go error
comparisons such as `errors.Is(err, protocol.ErrInvalidReport)`. Error-number
ranges identify the responsible layer.

| Code | Constant | Meaning |
|---|---|---|
| `E0101` | `config.ErrInvalidVendorID` | Invalid configured USB vendor ID |
| `E0102` | `config.ErrInvalidProductID` | Invalid configured USB product ID |
| `E0103` | `config.ErrInvalidTimeout` | Invalid configured request timeout |
| `E0104` | `config.ErrInvalidHexID` | Invalid generic 16-bit hexadecimal ID |
| `E0201` | `hid.ErrEnumerate` | HID enumeration failed |
| `E0202` | `hid.ErrDeviceNotFound` | Matching PRHP HID device was not found |
| `E0203` | `hid.ErrOpen` | HID device open failed |
| `E0204` | `hid.ErrShortReport` | HID read or write had an invalid length |
| `E0205` | `hid.ErrClosed` | Write attempted after transport shutdown |
| `E0206` | `hid.ErrRead` | HID read failed |
| `E0207` | `hid.ErrWrite` | HID write failed |
| `E0208` | `hid.ErrClose` | HID close failed |
| `E0301` | `hid.ErrInvalidTimeout` | Caller timeout is not positive |
| `E0302` | `hid.ErrSequenceExhausted` | No request sequence is available |
| `E0303` | `hid.ErrUnknownSequence` | Response sequence is unknown, expired, or consumed |
| `E0304` | `hid.ErrUnknownSignal` | No handler is registered for a signal opcode |
| `E0305` | `hid.ErrCallerCacheInit` | Caller cache construction failed |
| `E0401` | `protocol.ErrInvalidReport` | PRHP report shape or header is invalid |
| `E0402` | `protocol.ErrInvalidType` | PRHP message type value is invalid |
| `E0403` | `protocol.ErrUnexpectedCode` | Response opcode does not match the request |
| `E0404` | `protocol.ErrUnexpectedType` | Valid message has the wrong type for the operation |
| `E0405` | `protocol.ErrUnknownCard` | Card-state signal contains an unknown card type |
| `E0406` | `protocol.ErrCommandFailed` | Device returned a non-success PRHP status |
| `E0501` | `aimeio.ErrTimeout` | Request timed out |
| `E0502` | `aimeio.ErrDisconnected` | Service stopped without a transport error |
| `E0503` | `aimeio.ErrContextDone` | Request context was canceled or expired |
| `E0504` | `aimeio.ErrUnsupportedVersion` | Connected reader uses an unsupported PRHP version |
