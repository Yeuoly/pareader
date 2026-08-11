# RFC 0001: PA Reader HID Protocol, Core Specification

| Field | Value |
|---|---|
| Category | Standards Track |
| Status | Draft |
| Version | 0.1.0 |
| Updated | 2026-08-11 |

## Status of This Memo

This document specifies version 0.1 of the PA Reader HID Protocol (PRHP).
Distribution of this memo is unlimited.

## Abstract

PRHP is a compact binary protocol between a Host and an NFC Reader over one
vendor-defined USB HID interface. Host commands use complete HID Output
reports. Command responses and Reader-originated card-state signals use
complete HID Input reports.

Every message explicitly declares whether it is a request, response, or
one-way signal. Responses copy a non-zero 16-bit request sequence. Signals use
no sequence. Version 0.1 defines protocol-version query, card-state delivery,
and an LED compatibility signal. PN532 commands and other NFC-controller details never cross
the HID boundary.

## 1. Conventions

The key words `MUST`, `MUST NOT`, `REQUIRED`, `SHOULD`, `SHOULD NOT`, and `MAY`
are to be interpreted as described in BCP 14.

Unless stated otherwise:

- an octet is eight bits;
- multi-octet integers are unsigned and little-endian;
- offsets are zero-based;
- reserved and padding octets are transmitted as zero.

## 2. Roles and Layer Boundary

The Host opens the PRHP HID interface, sends commands, correlates responses,
and dispatches signals. It may expose a synchronous API to another process.

The Reader owns the NFC driver, observes the physical card, emits normalized
card state, and executes Host commands. The Reader may execute incoming
commands serially and does not need a request-correlation map.

The transport and correlation layers handle complete opaque reports. Only an
opcode-specific codec interprets bytes after the common header. PN532 frames,
I2C traffic, authentication exchanges, and controller-specific status values
MUST NOT appear in PRHP.

PRHP is independent of the separate SEGA CDC protocol and CardIO HID binding.
A composite USB device MAY expose all three at once. The CardIO collection MUST
be placed on its own HID interface and MUST NOT be combined with the PRHP
collection.

## 3. USB HID Binding

The Reader exposes one PRHP HID interface:

| Property | Value |
|---|---:|
| USB class | `0x03` (HID) |
| Subclass | `0x00` |
| Protocol | `0x00` |
| Usage Page | `0xFF50` |
| Usage | `0x01` |
| Input report | 64 octets |
| Output report | 64 octets |
| Report IDs | Not used |

USB VID, PID, strings, and serial number are deployment identifiers. A Host
SHOULD match configured VID and PID together with Usage Page and Usage.

Some HID APIs require a leading zero when writing an interface without Report
IDs. That API byte is not part of the 64-octet PRHP report.

Each HID report contains exactly one PRHP message. PRHP has no fragmentation or
length field. The opcode determines the effective body length; remaining
octets are zero padding and are ignored by the receiver.

## 4. Common Header

Every message begins with this five-octet header:

| Offset | Size | Field | Description |
|---:|---:|---|---|
| 0 | 1 | `type` | Message type |
| 1 | 1 | `opcode` | Operation code |
| 2 | 2 | `sequence` | Request-response correlation |
| 4 | 1 | `status` | Response status; zero otherwise |

The operation body starts at offset 5.

### 4.1. Message types

| Value | Name | Direction | Meaning |
|---:|---|---|---|
| `0x01` | `REQUEST` | Host to Reader | Requires one `RESPONSE` |
| `0x02` | `RESPONSE` | Reader to Host | Result of one `REQUEST` |
| `0x03` | `SIGNAL` | Either direction | One-way message |

A `REQUEST` has non-zero `sequence` and zero `status`. A `RESPONSE` copies the
request's non-zero `sequence`. A `SIGNAL` has zero `sequence` and zero `status`.

Routing is selected only by `type`. Implementations MUST NOT infer a message
type from sequence, status, opcode, direction, or body contents.

### 4.2. Response routing

The Host keeps in-flight request sequences unique. A received `RESPONSE` is
routed as a complete raw report to the caller registered for its sequence. A
received `SIGNAL` is routed as a complete raw report to the handler registered
for its opcode.

PRHP carries no timeout value. A Host chooses its local timeout. A response for
an unknown or expired sequence is ignored.

## 5. Status Codes

| Value | Name | Meaning |
|---:|---|---|
| `0x00` | `SUCCESS` | Operation completed |
| `0x01` | `UNKNOWN_OPCODE` | Opcode is not implemented |
| `0x02` | `INVALID_MESSAGE` | Message shape is invalid |
| `0x03`-`0xFF` | Reserved | Future allocation |

Error responses have no operation body in version 0.1.

## 6. Opcode Registry

| Value | Name | Mode | Direction |
|---:|---|---|---|
| `0x01` | `GET_VERSION` | Request-response | Host to Reader |
| `0x02` | `CARD_STATE` | Signal | Reader to Host |
| `0x03` | `SET_LED` | Signal | Host to Reader |
| `0x04`-`0xFF` | Reserved | N/A | N/A |

An opcode used with the wrong message type is discarded. An unknown request
produces `UNKNOWN_OPCODE`; an unknown signal is discarded.

## 7. GET_VERSION (`0x01`)

The request body is empty. A successful response body contains:

| Body offset | Size | Field |
|---:|---:|---|
| 0 | 1 | `version_major` |
| 1 | 1 | `version_minor` |

An implementation of this document returns `0x00, 0x01`.

## 8. CARD_STATE (`0x02`)

`CARD_STATE` is a Reader-to-Host signal. It replaces a Host-driven card-read
command: version 0.1 defines no `READ_CARD` request.

The Reader observes the card independently. It stores one current observation.
When a new observation differs from the stored observation, the Reader replaces
the stored value and emits one `CARD_STATE`. If it is equal, the Reader emits
nothing.

Consequently:

- a held card produces one signal, not repeated signals;
- the first no-card observation after a card produces one `NONE` signal;
- later no-card observations produce nothing until a card appears;
- replacing one card with another produces one signal for the new card.

The first body octet is `card_type`:

| Value | Name | Meaning |
|---:|---|---|
| `0x00` | `NONE` | No supported PRHP card |
| `0x01` | `MIFARE` | Aime/Banapass MIFARE result |
| `0x02` | `FELICA` | FeliCa result |

### 8.1. NONE

`NONE` has no fields after `card_type`.

```text
03 02 00 00 00 00 00 ... 00
```

### 8.2. MIFARE

| Body offset | Size | Field | Description |
|---:|---:|---|---|
| 0 | 1 | `card_type` | `0x01` |
| 1 | 10 | `luid` | Packed-BCD 20-digit access code |
| 11 | 16 | `block_1` | Raw MIFARE Classic block 1 |
| 27 | 16 | `block_2` | Raw MIFARE Classic block 2 |

Each LUID nibble is 0 through 9. An all-zero LUID means that the blocks are
available but an AimeIO-compatible access code is not.

### 8.3. FeliCa

| Body offset | Size | Field | Description |
|---:|---:|---|---|
| 0 | 1 | `card_type` | `0x02` |
| 1 | 8 | `idm` | IDm in card byte order |

The IDm is an octet string, not a PRHP integer.

### 8.4. Host state

The Host stores the most recently received valid state using an atomic
replacement. `NONE` replaces any previous card. A malformed signal is ignored
and does not modify the stored state.

## 9. SET_LED (`0x03`)

`SET_LED` is a Host-to-Reader signal:

| Body offset | Size | Field |
|---:|---:|---|
| 0 | 1 | `red` |
| 1 | 1 | `green` |
| 2 | 1 | `blue` |

Each intensity is 0 through 255. The Reader accepts and ignores the values;
physical LED output is not part of PRHP. No response is emitted.

## 10. Failure and Extensibility

The Reader keeps no response history, sequence session, retransmission cache,
or timeout state. Physical reader state and the current card observation are
not request-correlation state.

Future specifications may allocate reserved opcodes, statuses, and card types.
Each new opcode defines its message type, direction, effective body length,
codec, and failure behavior independently. The common transport continues to
pass operation bytes unchanged.
