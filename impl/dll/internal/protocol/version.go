package protocol

type Version struct {
	Major byte
	Minor byte
}

func EncodeGetVersion(sequence uint16) []byte {
	return NewRequest(OpcodeGetVersion, sequence)
}

func DecodeGetVersionResponse(raw []byte) (Version, error) {
	if _, err := validateResponse(raw, OpcodeGetVersion); err != nil {
		return Version{}, err
	}
	return Version{Major: raw[5], Minor: raw[6]}, nil
}
