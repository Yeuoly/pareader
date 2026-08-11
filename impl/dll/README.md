# PA Reader Host

This Go module contains the PRHP host, terminal reader application, and Windows
AimeIO 1.0 DLL. It opens only the PRHP HID interface with Usage Page `0xFF50`;
Spice2x opens the separate CardIO interface itself.

## Runtime model

One goroutine continuously reads HID Input reports. The router sends
`RESPONSE` reports to the sequence caller and sends `CARD_STATE` signals to the
card handler. The handler decodes the signal and atomically replaces one
in-memory value:

```go
router.Handle(protocol.OpcodeCardState, func(raw []byte) error {
    card, err := protocol.DecodeCardStateSignal(raw)
    if err != nil {
        return err
    }
    service.latest.Store(&card)
    return nil
})
```

There is no card-read request. `aime_io_nfc_poll` remains exported for ABI
compatibility but performs no transport operation. `aime_io_nfc_get_aime_id`,
`aime_io_nfc_get_mifare_card_id`, and `aime_io_nfc_get_felica_id` read the
latest state.

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
make dll
```

`make dll` targets Windows amd64 with cgo and needs a Windows cgo compiler such
as MinGW-w64 when run on another operating system. It emits `build/aimeio.dll`
and the generated C header.

| Environment variable | Default | Meaning |
|---|---|---|
| `PAREADER_VID` | `5041` | USB vendor ID in hexadecimal |
| `PAREADER_PID` | `5245` | USB product ID in hexadecimal |
| `PAREADER_SERIAL` | empty | Optional exact serial match |
| `PAREADER_TIMEOUT` | `1s` | Local request timeout for request-response commands |

