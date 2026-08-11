package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	ReportSize = 64
)

type MessageType byte

const (
	MessageRequest  MessageType = 0x01
	MessageResponse MessageType = 0x02
	MessageSignal   MessageType = 0x03
)

const (
	OpcodeGetVersion byte = 0x01
	OpcodeCardState  byte = 0x02
	OpcodeSetLED     byte = 0x03
)

type Status byte

const (
	StatusSuccess        Status = 0x00
	StatusUnknownOpcode  Status = 0x01
	StatusInvalidMessage Status = 0x02
)

var (
	ErrInvalidReport  = errors.New("invalid PRHP report")
	ErrInvalidType    = errors.New("invalid PRHP message type")
	ErrUnexpectedCode = errors.New("unexpected PRHP opcode")
	ErrUnexpectedType = errors.New("unexpected PRHP message type")
	ErrUnknownCard    = errors.New("unknown PRHP card type")
)

type Header struct {
	Type     MessageType
	Opcode   byte
	Sequence uint16
	Status   Status
}

type StatusError struct {
	Opcode byte
	Status Status
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("PRHP opcode 0x%02X failed with status 0x%02X", e.Opcode, byte(e.Status))
}

func DecodeHeader(raw []byte) (Header, error) {
	if len(raw) != ReportSize {
		return Header{}, fmt.Errorf("%w: got %d bytes, want %d", ErrInvalidReport, len(raw), ReportSize)
	}

	header := Header{
		Type:     MessageType(raw[0]),
		Opcode:   raw[1],
		Sequence: binary.LittleEndian.Uint16(raw[2:4]),
		Status:   Status(raw[4]),
	}

	switch header.Type {
	case MessageRequest:
		if header.Sequence == 0 || header.Status != StatusSuccess {
			return Header{}, fmt.Errorf("%w: invalid REQUEST header", ErrInvalidReport)
		}
	case MessageResponse:
		if header.Sequence == 0 {
			return Header{}, fmt.Errorf("%w: invalid RESPONSE header", ErrInvalidReport)
		}
	case MessageSignal:
		if header.Sequence != 0 || header.Status != StatusSuccess {
			return Header{}, fmt.Errorf("%w: invalid SIGNAL header", ErrInvalidReport)
		}
	default:
		return Header{}, fmt.Errorf("%w: 0x%02X", ErrInvalidType, byte(header.Type))
	}

	return header, nil
}

func NewRequest(opcode byte, sequence uint16) []byte {
	raw := make([]byte, ReportSize)
	raw[0] = byte(MessageRequest)
	raw[1] = opcode
	binary.LittleEndian.PutUint16(raw[2:4], sequence)
	return raw
}

func NewSignal(opcode byte) []byte {
	raw := make([]byte, ReportSize)
	raw[0] = byte(MessageSignal)
	raw[1] = opcode
	return raw
}

func validateResponse(raw []byte, opcode byte) (Header, error) {
	header, err := DecodeHeader(raw)
	if err != nil {
		return Header{}, err
	}
	if header.Type != MessageResponse {
		return Header{}, fmt.Errorf("%w: got 0x%02X, want RESPONSE", ErrUnexpectedType, byte(header.Type))
	}
	if header.Opcode != opcode {
		return Header{}, fmt.Errorf("%w: got 0x%02X, want 0x%02X", ErrUnexpectedCode, header.Opcode, opcode)
	}
	if header.Status != StatusSuccess {
		return Header{}, &StatusError{Opcode: header.Opcode, Status: header.Status}
	}
	return header, nil
}
