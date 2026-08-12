package aimeio

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/Yeuoly/pareader/impl/dll/internal/hid"
	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

const defaultRetryDelay = 500 * time.Millisecond

type openDevice func() (hid.Transport, hid.DeviceInfo, error)

// Service supervises PRHP HID connections for the lifetime of the host process.
type Service struct {
	open       openDevice
	timeout    time.Duration
	retryDelay time.Duration
	ctx        context.Context
	cancel     context.CancelFunc
	active     atomic.Pointer[Session]
	deviceInfo atomic.Pointer[hid.DeviceInfo]
	lastErr    atomic.Pointer[readFailure]
	stopped    chan struct{}
	closed     atomic.Bool
}

func NewService(config hid.DeviceConfig, timeout time.Duration) *Service {
	return newService(func() (hid.Transport, hid.DeviceInfo, error) {
		return hid.Open(config)
	}, timeout, defaultRetryDelay)
}

func newService(open openDevice, timeout, retryDelay time.Duration) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{
		open:       open,
		timeout:    timeout,
		retryDelay: retryDelay,
		ctx:        ctx,
		cancel:     cancel,
		stopped:    make(chan struct{}),
	}
	go service.run()
	return service
}

func (s *Service) run() {
	defer close(s.stopped)

	for {
		if s.ctx.Err() != nil {
			return
		}

		transport, info, err := s.open()
		if err != nil {
			s.storeError(err)
			if !s.waitRetry() {
				return
			}
			continue
		}

		session := newSession(transport, s.timeout)
		if err := session.VerifyVersion(s.ctx); err != nil {
			s.storeError(err)
			_ = session.Close()
			<-session.Done()
			if !s.waitRetry() {
				return
			}
			continue
		}

		s.deviceInfo.Store(&info)
		s.lastErr.Store(nil)
		s.active.Store(session)

		select {
		case <-session.Done():
		case <-s.ctx.Done():
			_ = session.Close()
			<-session.Done()
		}

		s.active.CompareAndSwap(session, nil)
		s.deviceInfo.Store(nil)
		s.storeError(session.Err())
		_ = session.Close()

		if !s.waitRetry() {
			return
		}
	}
}

func (s *Service) waitRetry() bool {
	if s.ctx.Err() != nil {
		return false
	}
	timer := time.NewTimer(s.retryDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *Service) CurrentCard() protocol.Card {
	session := s.active.Load()
	if session == nil {
		return protocol.Card{Type: protocol.CardNone}
	}
	return session.CurrentCard()
}

func (s *Service) Err() error {
	if s.active.Load() != nil {
		return nil
	}
	failure := s.lastErr.Load()
	if failure == nil || failure.err == nil {
		return ErrDisconnected
	}
	return failure.err
}

func (s *Service) DeviceInfo() (hid.DeviceInfo, bool) {
	info := s.deviceInfo.Load()
	if info == nil {
		return hid.DeviceInfo{}, false
	}
	return *info, true
}

func (s *Service) AimeID() ([10]byte, bool) {
	session := s.active.Load()
	if session == nil {
		return [10]byte{}, false
	}
	return session.AimeID()
}

func (s *Service) MIFAREBlocks() ([32]byte, bool) {
	session := s.active.Load()
	if session == nil {
		return [32]byte{}, false
	}
	return session.MIFAREBlocks()
}

func (s *Service) FeliCaID() (uint64, bool) {
	session := s.active.Load()
	if session == nil {
		return 0, false
	}
	return session.FeliCaID()
}

func (s *Service) SetLED(red, green, blue byte) error {
	session := s.active.Load()
	if session == nil {
		return ErrDisconnected
	}
	return session.SetLED(red, green, blue)
}

func (s *Service) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		<-s.stopped
		return nil
	}
	s.cancel()
	if session := s.active.Load(); session != nil {
		_ = session.Close()
	}
	<-s.stopped
	return nil
}

func (s *Service) storeError(err error) {
	if err != nil {
		s.lastErr.Store(&readFailure{err: err})
	}
}
