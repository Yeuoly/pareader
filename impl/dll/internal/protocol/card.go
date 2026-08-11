package protocol

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
		return Card{}, ErrUnexpectedType
	}
	if header.Opcode != OpcodeCardState {
		return Card{}, ErrUnexpectedCode
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
		return Card{}, ErrUnknownCard
	}
}

func validateLUID(luid [10]byte) error {
	for _, value := range luid {
		if value>>4 > 9 || value&0x0F > 9 {
			return ErrInvalidReport
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
	const hex = "0123456789ABCDEF"
	result := make([]byte, len(idm)*2)
	for i, value := range idm {
		result[i*2] = hex[value>>4]
		result[i*2+1] = hex[value&0x0F]
	}
	return string(result)
}
