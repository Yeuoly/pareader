package hid

import (
	"bytes"
	"sync/atomic"
	"time"

	freelru "github.com/elastic/go-freelru"

	"github.com/Yeuoly/pareader/impl/dll/internal/errcode"
)

const callerCapacity = 1024

const (
	ErrInvalidTimeout    errcode.Code = "E0301"
	ErrSequenceExhausted errcode.Code = "E0302"
	ErrUnknownSequence   errcode.Code = "E0303"
	ErrCallerCacheInit   errcode.Code = "E0305"
)

type Caller struct {
	next  atomic.Uint32
	calls *freelru.SyncedLRU[uint16, *pendingCall]
}

type pendingCall struct {
	response chan []byte
	consumed atomic.Bool
}

func NewCaller() *Caller {
	calls, err := freelru.NewSynced[uint16, *pendingCall](callerCapacity, func(sequence uint16) uint32 {
		return uint32(sequence)
	})
	if err != nil {
		panic(ErrCallerCacheInit)
	}
	return &Caller{calls: calls}
}

func (c *Caller) Allocate(timeout time.Duration) (uint16, <-chan []byte, error) {
	if timeout <= 0 {
		return 0, nil, ErrInvalidTimeout
	}

	for attempts := 0; attempts < 0xFFFF; attempts++ {
		sequence := uint16(c.next.Add(1))
		if sequence == 0 {
			continue
		}

		if _, found := c.calls.Get(sequence); found {
			continue
		}

		call := &pendingCall{response: make(chan []byte, 1)}
		c.calls.AddWithLifetime(sequence, call, timeout)
		return sequence, call.response, nil
	}

	return 0, nil, ErrSequenceExhausted
}

func (c *Caller) Consume(sequence uint16, raw []byte) error {
	call, found := c.calls.Get(sequence)
	if !found || !call.consumed.CompareAndSwap(false, true) {
		return ErrUnknownSequence
	}

	c.calls.Remove(sequence)
	call.response <- bytes.Clone(raw)
	return nil
}
