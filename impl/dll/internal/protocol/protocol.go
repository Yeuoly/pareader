package protocol

import (
	"encoding/binary"

	"github.com/Yeuoly/pareader/impl/dll/internal/errcode"
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

const (
	ErrInvalidReport  errcode.Code = "E0401"
	ErrInvalidType    errcode.Code = "E0402"
	ErrUnexpectedCode errcode.Code = "E0403"
	ErrUnexpectedType errcode.Code = "E0404"
	ErrUnknownCard    errcode.Code = "E0405"
	ErrCommandFailed  errcode.Code = "E0406"
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
	return ErrCommandFailed.Error()
}

func (e *StatusError) Is(target error) bool {
	code, ok := target.(errcode.Code)
	return ok && code == ErrCommandFailed
}

func DecodeHeader(raw []byte) (Header, error) {
	if len(raw) != ReportSize {
		return Header{}, ErrInvalidReport
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
			return Header{}, ErrInvalidReport
		}
	case MessageResponse:
		if header.Sequence == 0 {
			return Header{}, ErrInvalidReport
		}
	case MessageSignal:
		if header.Sequence != 0 || header.Status != StatusSuccess {
			return Header{}, ErrInvalidReport
		}
	default:
		return Header{}, ErrInvalidType
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
		return Header{}, ErrUnexpectedType
	}
	if header.Opcode != opcode {
		return Header{}, ErrUnexpectedCode
	}
	if header.Status != StatusSuccess {
		return Header{}, &StatusError{Opcode: header.Opcode, Status: header.Status}
	}
	return header, nil
}
