package protocol

import "fmt"

type CardType byte

const (
	CardNone   CardType = 0x00
	CardMIFARE CardType = 0x01
	CardFeliCa CardType = 0x02
)

type Card struct {
	Type   CardType
	LUID   [10]byte
	Blocks [32]byte
	IDm    [8]byte
}

func DecodeCardStateSignal(raw []byte) (Card, error) {
	header, err := DecodeHeader(raw)
	if err != nil {
		return Card{}, err
	}
	if header.Type != MessageSignal {
		return Card{}, fmt.Errorf("%w: got 0x%02X, want SIGNAL", ErrUnexpectedType, byte(header.Type))
	}
	if header.Opcode != OpcodeCardState {
		return Card{}, fmt.Errorf("%w: got 0x%02X, want 0x%02X", ErrUnexpectedCode, header.Opcode, OpcodeCardState)
	}

	card := Card{Type: CardType(raw[5])}
	switch card.Type {
	case CardNone:
		return card, nil
	case CardMIFARE:
		copy(card.LUID[:], raw[6:16])
		copy(card.Blocks[:], raw[16:48])
		if err := validateLUID(card.LUID); err != nil {
			return Card{}, err
		}
		return card, nil
	case CardFeliCa:
		copy(card.IDm[:], raw[6:14])
		return card, nil
	default:
		return Card{}, fmt.Errorf("%w: 0x%02X", ErrUnknownCard, byte(card.Type))
	}
}

func validateLUID(luid [10]byte) error {
	for _, value := range luid {
		if value>>4 > 9 || value&0x0F > 9 {
			return fmt.Errorf("%w: invalid packed-BCD LUID", ErrInvalidReport)
		}
	}
	return nil
}

func FormatLUID(luid [10]byte) string {
	result := make([]byte, 20)
	for i, value := range luid {
		result[i*2] = '0' + value>>4
		result[i*2+1] = '0' + value&0x0F
	}
	return string(result)
}

func FormatIDm(idm [8]byte) string {
	return fmt.Sprintf("%02X%02X%02X%02X%02X%02X%02X%02X",
		idm[0], idm[1], idm[2], idm[3], idm[4], idm[5], idm[6], idm[7])
}
