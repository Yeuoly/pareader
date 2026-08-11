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
	ErrTimeout      errcode.Code = "E0501"
	ErrDisconnected errcode.Code = "E0502"
	ErrContextDone  errcode.Code = "E0503"
)

type readFailure struct {
	err error
}

type Service struct {
	transport hid.Transport
	caller    *hid.Caller
	timeout   time.Duration
	latest    atomic.Pointer[protocol.Card]
	readErr   atomic.Pointer[readFailure]
	done      chan struct{}
}

func NewService(transport hid.Transport, timeout time.Duration) *Service {
	caller := hid.NewCaller()
	service := &Service{
		transport: transport,
		caller:    caller,
		timeout:   timeout,
		done:      make(chan struct{}),
	}
	service.latest.Store(&protocol.Card{Type: protocol.CardNone})

	router := hid.NewRouter(caller)
	router.Handle(protocol.OpcodeCardState, func(raw []byte) error {
		card, err := protocol.DecodeCardStateSignal(raw)
		if err != nil {
			return err
		}
		service.latest.Store(&card)
		return nil
	})
	go func() {
		err := hid.ReadLoop(transport, router)
		service.latest.Store(&protocol.Card{Type: protocol.CardNone})
		service.readErr.Store(&readFailure{err: err})
		close(service.done)
	}()

	return service
}

func (s *Service) Version(ctx context.Context) (protocol.Version, error) {
	sequence, response, err := s.caller.Allocate(s.timeout)
	if err != nil {
		return protocol.Version{}, err
	}
	if err := s.transport.Write(protocol.EncodeGetVersion(sequence)); err != nil {
		return protocol.Version{}, err
	}

	select {
	case raw := <-response:
		return protocol.DecodeGetVersionResponse(raw)
	case <-time.After(s.timeout):
		return protocol.Version{}, ErrTimeout
	case <-ctx.Done():
		return protocol.Version{}, ErrContextDone
	case <-s.done:
		return protocol.Version{}, s.disconnectedError()
	}
}

func (s *Service) CurrentCard() protocol.Card {
	card := s.latest.Load()
	if card == nil {
		return protocol.Card{Type: protocol.CardNone}
	}
	return *card
}

func (s *Service) Err() error {
	select {
	case <-s.done:
		return s.disconnectedError()
	default:
		return nil
	}
}

func (s *Service) AimeID() ([10]byte, bool) {
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

func (s *Service) MIFAREBlocks() ([32]byte, bool) {
	card := s.CurrentCard()
	return card.Blocks, card.Type == protocol.CardMIFARE
}

func (s *Service) FeliCaID() (uint64, bool) {
	card := s.CurrentCard()
	if card.Type != protocol.CardFeliCa {
		return 0, false
	}
	return binary.BigEndian.Uint64(card.IDm[:]), true
}

func (s *Service) SetLED(red, green, blue byte) error {
	return s.transport.Write(protocol.EncodeSetLED(red, green, blue))
}

func (s *Service) Close() error {
	return s.transport.Close()
}

func (s *Service) disconnectedError() error {
	failure := s.readErr.Load()
	if failure == nil || failure.err == nil {
		return ErrDisconnected
	}
	return failure.err
}
