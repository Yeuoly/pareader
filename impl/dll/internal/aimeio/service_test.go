package aimeio

import (
	"io"
	"testing"
	"time"

	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

type fakeTransport struct {
	writes chan []byte
	reads  chan []byte
	done   chan struct{}
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		writes: make(chan []byte, 1),
		reads:  make(chan []byte, 1),
		done:   make(chan struct{}),
	}
}

func (t *fakeTransport) Read() ([]byte, error) {
	select {
	case raw := <-t.reads:
		return raw, nil
	case <-t.done:
		return nil, io.EOF
	}
}

func (t *fakeTransport) Write(raw []byte) error {
	t.writes <- raw
	return nil
}

func (t *fakeTransport) Close() error {
	select {
	case <-t.done:
	default:
		close(t.done)
	}
	return nil
}

func TestCardStateSignalUpdatesCache(t *testing.T) {
	transport := newFakeTransport()
	service := NewService(transport, time.Second)
	defer service.Close()

	signal := make([]byte, protocol.ReportSize)
	signal[0] = byte(protocol.MessageSignal)
	signal[1] = protocol.OpcodeCardState
	signal[5] = byte(protocol.CardFeliCa)
	copy(signal[6:14], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	transport.reads <- signal

	deadline := time.Now().Add(time.Second)
	for service.CurrentCard().Type != protocol.CardFeliCa && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := protocol.FormatIDm(service.CurrentCard().IDm); got != "0102030405060708" {
		t.Fatalf("unexpected IDm %s", got)
	}

	none := make([]byte, protocol.ReportSize)
	none[0] = byte(protocol.MessageSignal)
	none[1] = protocol.OpcodeCardState
	transport.reads <- none
	for service.CurrentCard().Type != protocol.CardNone && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.CurrentCard().Type != protocol.CardNone {
		t.Fatal("card state was not cleared")
	}
}
