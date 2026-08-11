# Implementation Architecture

This directory contains the ESP32-S3 firmware and Go host implementation for
the protocols under [`spec/`](../spec/README.md). The RFC defines wire
behavior; this document defines package boundaries and concrete implementation
patterns.

## Layout

```text
impl/
├── firmware/
│   └── main/
│       ├── app_main.c       NFC observation loop
│       ├── usb_device.c     composite USB descriptors and endpoints
│       ├── prhp.c           PRHP codec and command dispatcher
│       ├── cardio.c         CardIO state encoder
│       ├── sega.c           SEGA CDC protocol
│       ├── reader.c         card semantics
│       └── pn532.c          PN532 I2C driver
└── dll/
    ├── cmd/aimeio/          Windows AimeIO exports
    ├── cmd/reader/          terminal reader application
    └── internal/
        ├── aimeio/          current card state and AimeIO behavior
        ├── hid/             transport, caller, and router
        └── protocol/        PRHP codecs
```

## USB boundary

The firmware is one composite USB device with three protocol functions:

```text
USB interfaces 0 and 1   CDC pair, SEGA protocol
USB interface 2          PRHP, Usage Page 0xFF50
USB interface 3          CardIO, Usage Page 0xFFCA
```

The HID functions are TinyUSB HID instances 0 and 1, respectively. They are
two independent USB HID interfaces. Each interface has
one top-level application collection and its own interrupt IN endpoint. Only
PRHP has an interrupt OUT endpoint. Do not combine PRHP and CardIO as two
collections in one HID interface.

The DLL enumerates VID/PID plus Usage Page `0xFF50` and Usage `0x01`, so it
cannot accidentally open the CardIO path. Spice2x finds CardIO by Usage Page
`0xFFCA` and never passes through the DLL.

## Protocol layering

Only `internal/protocol` and `prhp.c` know PRHP byte offsets. HID transport
handles complete 64-byte byte strings. It does not know card types, PN532, or
AimeIO.

```go
type Transport interface {
    Read() ([]byte, error)
    Write([]byte) error
}
```

There is no common typed request payload. Each operation owns its codec:

```go
EncodeGetVersion(sequence uint16) []byte
DecodeGetVersionResponse(raw []byte) (Version, error)
DecodeCardStateSignal(raw []byte) (Card, error)
EncodeSetLED(red, green, blue byte) []byte
```

## HID output

`Write` validates and copies one report into a bounded queue. It has no result
channel and does not wait for hidapi:

```go
func (d *Device) Write(raw []byte) error {
    if len(raw) != protocol.ReportSize {
        return protocol.ErrInvalidReport
    }

    select {
    case d.writes <- bytes.Clone(raw):
        return nil
    case <-d.done:
        return ErrClosed
    }
}
```

One goroutine owns physical HID writes and preserves queue order:

```go
func (d *Device) writeLoop() {
    for {
        select {
        case raw := <-d.writes:
            if err := d.writeReport(raw); err != nil {
                d.closeTransport()
                return
            }
        case <-d.done:
            return
        }
    }
}
```

There is no write mutex and no per-write completion channel.

## Request-response caller

The caller is used only by actual request-response commands such as
`GET_VERSION`. Its public operations are:

```go
Allocate(timeout time.Duration) (sequence uint16, response <-chan []byte, err error)
Consume(sequence uint16, raw []byte) error
```

It uses `github.com/elastic/go-freelru` as a concurrency-safe, time-based LRU.
It does not start a cache cleanup goroutine. Expiration is observed lazily, and
the business layer never deletes a timed-out entry manually.

```go
type Caller struct {
    next  atomic.Uint32
    calls *freelru.SyncedLRU[uint16, *pendingCall]
}

type pendingCall struct {
    response chan []byte
    consumed atomic.Bool
}

func (c *Caller) Allocate(timeout time.Duration) (uint16, <-chan []byte, error) {
    if timeout <= 0 {
        return 0, nil, ErrInvalidTimeout
    }

    for attempts := 0; attempts < 0xffff; attempts++ {
        sequence := uint16(c.next.Add(1))
        if sequence == 0 {
            continue
        }

        if _, found := c.calls.Get(sequence); found {
            continue
        }

        call := &pendingCall{response: make(chan []byte, 1)}
        c.calls.AddWithLifetime(sequence, call, timeout)
        return sequence, call.response, nil
    }
    return 0, nil, ErrSequenceExhausted
}

func (c *Caller) Consume(sequence uint16, raw []byte) error {
    call, found := c.calls.Get(sequence)
    if !found || !call.consumed.CompareAndSwap(false, true) {
        return ErrUnknownSequence
    }

    c.calls.Remove(sequence)
    call.response <- bytes.Clone(raw)
    return nil
}
```

The operation decides its own timeout with `select`; the caller does not expose
a `Call` method:

```go
sequence, response, err := caller.Allocate(timeout)
if err != nil {
    return err
}
if err := transport.Write(protocol.EncodeGetVersion(sequence)); err != nil {
    return err
}

select {
case raw := <-response:
    return protocol.DecodeGetVersionResponse(raw)
case <-time.After(timeout):
    return ErrTimeout
case <-ctx.Done():
    return ctx.Err()
}
```

One-way messages bypass the caller:

```go
return transport.Write(protocol.EncodeSetLED(red, green, blue))
```

## HID input router

One goroutine continuously reads Input reports. The router examines only the
explicit message type:

```go
func (r *Router) Dispatch(raw []byte) error {
    header, err := protocol.DecodeHeader(raw)
    if err != nil {
        return err
    }

    switch header.Type {
    case protocol.MessageResponse:
        return r.caller.Consume(header.Sequence, raw)
    case protocol.MessageSignal:
        handler, found := r.signals[header.Opcode]
        if !found {
            return ErrUnknownSignal
        }
        return handler(raw)
    default:
        return protocol.ErrUnexpectedType
    }
}
```

Sequence zero is an invariant check, not a hidden packet-type discriminator.
The router chooses response correlation or signal dispatch from `type` before
using sequence or opcode.

## AimeIO card state

The firmware originates `CARD_STATE`. The DLL signal handler decodes the
operation body and atomically replaces one immutable fixed-size card value:

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

The AimeIO ABI remains synchronous, but polling does not create a HID command:

```go
//export aime_io_nfc_poll
func aime_io_nfc_poll(unitNo uint8) C.HRESULT {
    if unitNo != 0 || currentService() == nil {
        return C.E_FAIL
    }
    return C.S_OK
}
```

The getters load `latest` and convert only the requested representation:

```go
func (s *Service) FeliCaID() (uint64, bool) {
    card := s.CurrentCard()
    if card.Type != protocol.CardFeliCa {
        return 0, false
    }
    return binary.BigEndian.Uint64(card.IDm[:]), true
}
```

No polling goroutine, card request, per-card channel, or service mutex is
needed. On HID disconnection, the service replaces the card with `NONE`.

## Firmware card observation

The firmware stores exactly one raw observation. About every 500 ms it reads
the PN532 and compares the result with that value:

```c
reader_card_t current = {0};

reader_card_t observed;
if (reader_read_card(&observed) == ESP_OK &&
        memcmp(&current, &observed, sizeof(current)) != 0) {
    current = observed;
    prhp_publish_card_state(&current);
    cardio_publish_card_state(&current);
}
```

This single comparison defines both HID outputs:

- a new card is sent once;
- a held card produces no additional report;
- removal sends one empty state;
- repeated empty observations produce no report.

There is no second published-card value and no separate scan-state machine.

### PRHP encoding

`prhp_publish_card_state` produces one 64-byte Reader-to-Host signal:

```c
uint8_t signal[PRHP_REPORT_SIZE] = {0};
signal[0] = PRHP_SIGNAL;
signal[1] = PRHP_CARD_STATE;
signal[5] = card_type;
usb_device_send_prhp(signal);
```

Incoming PRHP commands are still dispatched synchronously. `GET_VERSION`
copies its request sequence into one response; `SET_LED` is a signal and emits
no response. `SET_LED` is accepted only for interface compatibility and has no
hardware effect. The firmware has no sequence map.

### CardIO encoding

CardIO is a separate Input-only HID interface. A four-byte MIFARE UID is
normalized to eight bytes, while FeliCa is copied unchanged:

```c
// MIFARE Report ID 1
uint8_t id[8] = {0xE0, 0x04, uid[0], uid[1], uid[2], uid[3], uid[0], uid[1]};

// FeliCa Report ID 2
memcpy(id, card->idm, 8);
```

Removal sends one all-zero report with Report ID zero. Spice2x consumes this
interface directly.

## Dependency rules

- `protocol` has no dependency on HID, AimeIO, TUI, or PN532 code.
- `hid` may depend on `protocol` only for common report validation and headers.
- `aimeio` composes HID and operation codecs; it does not parse raw offsets.
- `cmd/aimeio` contains ABI conversion and delegates immediately to `aimeio`.
- `cmd/reader` displays `aimeio.Service` state; it does not own another reader.
- `pn532` contains controller and I2C behavior only.
- `reader` converts PN532 results into shared card semantics.
- `prhp`, `cardio`, and `sega` encode independent host protocols.
