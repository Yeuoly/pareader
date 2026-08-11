# PA Reader

PA Reader is an open-source NFC card reader implementation with native SEGA
reader protocol support over USB CDC. It supports Segatools and Spice2x
simultaneously. No mode switching is required: a Segatools game and a Spice2x
game can remain open at the same time and use the same physical reader. PA
Reader runs on an ESP32-S3, uses an Elechouse PN532 NFC Module V3 over I2C, and
exposes one composite USB device with three independent interfaces:

- CDC-ACM implementing the SEGA reader protocol;
- PRHP HID for the AimeIO DLL and reader application;
- CardIO HID for Spice2x.

The two HID functions are separate USB interfaces. PRHP and CardIO are not two
collections packed into one HID interface.

## Architecture

```text
SEGA host   <---- CDC-ACM ----+
AimeIO DLL  <---- PRHP HID ---+--- ESP32-S3 --- I2C --- PN532
Spice2x     <---- CardIO HID -+
```

The firmware scans the NFC field approximately every 500 ms and stores one
current card observation. It publishes only transitions:

- a newly observed card is sent once;
- a held card is not sent again;
- card removal is sent once as an empty state;
- repeated empty scans produce no traffic.

The same transition is encoded independently for both HID interfaces. The
PRHP DLL updates an atomic in-memory card state. Spice2x consumes CardIO
directly; the DLL does not open or process the CardIO interface.

## USB interfaces

| Consumer | USB function | Usage Page | Usage | Direction |
|---|---|---:|---:|---|
| SEGA-compatible host | CDC-ACM | N/A | N/A | Bidirectional |
| PA Reader DLL | PRHP HID | `0xFF50` | `0x01` | Bidirectional |
| Spice2x CardIO | CardIO HID | `0xFFCA` | `0x01` | Reader to host |

The default device identity is VID `0x5041` (`PA`) and PID `0x5245` (`RE`).
These are project-local identifiers, not copied from another reader.

### PRHP

PRHP uses fixed 64-byte reports without Report IDs. It has explicit `REQUEST`,
`RESPONSE`, and `SIGNAL` message types. Card data is delivered by the
Reader-to-Host `CARD_STATE` signal; there is no `READ_CARD` command.

The optional AimeIO DLL keeps synchronous exported functions while its HID
reader and writer run in goroutines. `aime_io_nfc_poll` is a compatibility
no-op, and the card getters read the latest atomically stored state.

### CardIO

The CardIO HID interface has the standard `0xFFCA` top-level collection and two
input Report IDs:

| Report ID | Usage | Payload |
|---:|---:|---|
| `0x01` | `0x41` | Eight-byte e-amusement/MIFARE identifier |
| `0x02` | `0x42` | Eight-byte FeliCa IDm |

A four-byte MIFARE UID `u0 u1 u2 u3` is encoded as
`E0 04 u0 u1 u2 u3 u0 u1`. FeliCa uses the original eight-byte IDm. Card
removal emits one all-zero report.

## Hardware

### Requirements

- ESP32-S3 with native USB device support;
- Elechouse PN532 NFC Module V3;
- four wires for I2C and power.

Set the NFC Module V3 to I2C mode before applying power:

| Switch | Position |
|---|---|
| 1 | `ON` |
| 2 | `OFF` |

Default wiring:

| PN532 | ESP32-S3 |
|---|---:|
| `VCC` | `3V3` |
| `GND` | `GND` |
| `SDA` | `GPIO2` |
| `SCL` | `GPIO3` |

The PN532 uses 7-bit I2C address `0x24` at 100 kHz. SDA and SCL are
configurable under **PA Reader** in `idf.py menuconfig`. Use the ESP32-S3 native
USB port; a USB-to-UART bridge cannot expose these interfaces.

PN532 supports the project’s MIFARE and FeliCa paths. It does not provide the
ISO/IEC 15693 support used by some older e-amusement cards.

## Build

Firmware:

```sh
cd impl/firmware
idf.py set-target esp32s3
idf.py build
idf.py -p PORT flash monitor
```

Reader application:

```sh
cd impl/dll
go run ./cmd/reader
```

Windows AimeIO DLL:

```sh
cd impl/dll
go install mvdan.cc/garble@v0.14.2
make dll
```

A Windows cgo toolchain and Garble `v0.14.2` are required to build the compact
`build/aimeio.dll`. Use `make dll-plain` for a stripped build with runtime
diagnostics.

## Documentation

- [`spec/rfc-0001-prhp.md`](spec/rfc-0001-prhp.md): PRHP 0.1 wire protocol;
- [`impl/README.md`](impl/README.md): implementation boundaries and concrete
  code patterns;
- [`impl/firmware/README.md`](impl/firmware/README.md): firmware wiring,
  interfaces, and runtime;
- [`impl/dll/README.md`](impl/dll/README.md): DLL and reader application.

## Repository layout

```text
spec/                         Protocol specifications
impl/firmware/main/           ESP32-S3 firmware
impl/dll/internal/protocol/   PRHP codecs
impl/dll/internal/hid/        HID transport, caller, and router
impl/dll/internal/aimeio/     Atomic AimeIO card state
impl/dll/cmd/reader/          Reader application
impl/dll/cmd/aimeio/          Windows DLL exports
```
