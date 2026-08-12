package aimeio

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"time"

	"github.com/Yeuoly/pareader/impl/dll/internal/errcode"
	"github.com/Yeuoly/pareader/impl/dll/internal/hid"
	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

const (
	ErrTimeout            errcode.Code = "E0501"
	ErrDisconnected       errcode.Code = "E0502"
	ErrContextDone        errcode.Code = "E0503"
	ErrUnsupportedVersion errcode.Code = "E0504"
)

type readFailure struct {
	err error
}

// Session is one open and verified PRHP HID connection.
type Session struct {
	transport hid.Transport
	caller    *hid.Caller
	timeout   time.Duration
	latest    atomic.Pointer[protocol.Card]
	readErr   atomic.Pointer[readFailure]
	done      chan struct{}
}

func newSession(transport hid.Transport, timeout time.Duration) *Session {
	caller := hid.NewCaller()
	session := &Session{
		transport: transport,
		caller:    caller,
		timeout:   timeout,
		done:      make(chan struct{}),
	}
	session.latest.Store(&protocol.Card{Type: protocol.CardNone})

	router := hid.NewRouter(caller)
	router.Handle(protocol.OpcodeCardState, func(raw []byte) error {
		card, err := protocol.DecodeCardStateSignal(raw)
		if err != nil {
			return err
		}
		session.latest.Store(&card)
		return nil
	})

	go session.readLoop(router)
	return session
}

func (s *Session) readLoop(router *hid.Router) {
	err := hid.ReadLoop(s.transport, router)
	if err == nil {
		err = ErrDisconnected
	}
	s.latest.Store(&protocol.Card{Type: protocol.CardNone})
	s.readErr.Store(&readFailure{err: err})
	close(s.done)
}

func (s *Session) VerifyVersion(ctx context.Context) error {
	sequence, response, err := s.caller.Allocate(s.timeout)
	if err != nil {
		return err
	}
	if err := s.transport.Write(protocol.EncodeGetVersion(sequence)); err != nil {
		return err
	}

	timer := time.NewTimer(s.timeout)
	defer timer.Stop()

	select {
	case raw := <-response:
		version, err := protocol.DecodeGetVersionResponse(raw)
		if err != nil {
			return err
		}
		if version.Major != 0 || version.Minor != 1 {
			return ErrUnsupportedVersion
		}
		return nil
	case <-timer.C:
		return ErrTimeout
	case <-ctx.Done():
		return ErrContextDone
	case <-s.done:
		return s.Err()
	}
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) CurrentCard() protocol.Card {
	card := s.latest.Load()
	if card == nil {
		return protocol.Card{Type: protocol.CardNone}
	}
	return *card
}

func (s *Session) Err() error {
	select {
	case <-s.done:
		failure := s.readErr.Load()
		if failure == nil || failure.err == nil {
			return ErrDisconnected
		}
		return failure.err
	default:
		return nil
	}
}

func (s *Session) AimeID() ([10]byte, bool) {
	card := s.CurrentCard()
	if card.Type != protocol.CardMIFARE {
		return [10]byte{}, false
	}
	for _, value := range card.LUID {
		if value != 0 {
			return card.LUID, true
		}
	}
	return [10]byte{}, false
}

func (s *Session) MIFAREBlocks() ([32]byte, bool) {
	card := s.CurrentCard()
	return card.Blocks, card.Type == protocol.CardMIFARE
}

func (s *Session) FeliCaID() (uint64, bool) {
	card := s.CurrentCard()
	if card.Type != protocol.CardFeliCa {
		return 0, false
	}
	return binary.BigEndian.Uint64(card.IDm[:]), true
}

func (s *Session) SetLED(red, green, blue byte) error {
	return s.transport.Write(protocol.EncodeSetLED(red, green, blue))
}

func (s *Session) Close() error {
	return s.transport.Close()
}
