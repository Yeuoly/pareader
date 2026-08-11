package protocol

import (
	"encoding/binary"
	"testing"
)

func response(opcode byte, sequence uint16) []byte {
	raw := make([]byte, ReportSize)
	raw[0] = byte(MessageResponse)
	raw[1] = opcode
	binary.LittleEndian.PutUint16(raw[2:4], sequence)
	return raw
}

func TestCardStateMIFARE(t *testing.T) {
	raw := make([]byte, ReportSize)
	raw[0] = byte(MessageSignal)
	raw[1] = OpcodeCardState
	raw[5] = byte(CardMIFARE)
	copy(raw[6:16], []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0x01, 0x23, 0x45, 0x67, 0x89})
	for i := range 32 {
		raw[16+i] = byte(i)
	}

	card, err := DecodeCardStateSignal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatLUID(card.LUID); got != "01234567890123456789" {
		t.Fatalf("unexpected LUID %q", got)
	}
	if card.Blocks[31] != 31 {
		t.Fatalf("unexpected final block byte %d", card.Blocks[31])
	}
}

func TestSetLEDIsOneWay(t *testing.T) {
	raw := EncodeSetLED(0xFF, 0x40, 0)
	header, err := DecodeHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	if header.Type != MessageSignal || header.Sequence != 0 || raw[5] != 0xFF || raw[6] != 0x40 || raw[7] != 0 {
		t.Fatalf("unexpected SET_LED report: % X", raw[:8])
	}
}
