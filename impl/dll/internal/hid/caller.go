package hid

import (
	"bytes"
	"errors"
	"sync/atomic"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

const callerCapacity = 1024

var (
	ErrInvalidTimeout    = errors.New("invalid HID call timeout")
	ErrSequenceExhausted = errors.New("HID sequence space exhausted")
	ErrUnknownSequence   = errors.New("unknown or expired HID sequence")
)

type Caller struct {
	next  atomic.Uint32
	calls *ttlcache.Cache[uint16, chan []byte]
}

func NewCaller() *Caller {
	calls := ttlcache.New[uint16, chan []byte](
		ttlcache.WithCapacity[uint16, chan []byte](callerCapacity),
		ttlcache.WithDisableTouchOnHit[uint16, chan []byte](),
	)
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

		response := make(chan []byte, 1)
		_, found := c.calls.GetOrSet(
			sequence,
			response,
			ttlcache.WithTTL[uint16, chan []byte](timeout),
		)
		if !found {
			return sequence, response, nil
		}
	}

	return 0, nil, ErrSequenceExhausted
}

func (c *Caller) Consume(sequence uint16, raw []byte) error {
	item, found := c.calls.GetAndDelete(sequence)
	if !found {
		return ErrUnknownSequence
	}

	item.Value() <- bytes.Clone(raw)
	return nil
}
