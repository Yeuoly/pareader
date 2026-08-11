package protocol

func EncodeSetLED(red, green, blue byte) []byte {
	raw := NewSignal(OpcodeSetLED)
	raw[5], raw[6], raw[7] = red, green, blue
	return raw
}
