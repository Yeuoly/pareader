package hid

import (
	"errors"

	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

var ErrUnknownSignal = errors.New("unknown HID signal")

type SignalHandler func([]byte) error

type Router struct {
	caller  *Caller
	signals map[byte]SignalHandler
}

func NewRouter(caller *Caller) *Router {
	return &Router{
		caller:  caller,
		signals: make(map[byte]SignalHandler),
	}
}

// Handle must be called before the read loop starts.
func (r *Router) Handle(opcode byte, handler SignalHandler) {
	r.signals[opcode] = handler
}

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

func ReadLoop(transport Transport, router *Router) error {
	for {
		raw, err := transport.Read()
		if err != nil {
			return err
		}
		_ = router.Dispatch(raw)
	}
}
