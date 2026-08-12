package aimeio

import (
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/Yeuoly/pareader/impl/dll/internal/hid"
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
	select {
	case t.writes <- raw:
		return nil
	case <-t.done:
		return io.EOF
	}
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
	session := newSession(transport, time.Second)
	defer func() {
		_ = session.Close()
		<-session.Done()
	}()

	signal := make([]byte, protocol.ReportSize)
	signal[0] = byte(protocol.MessageSignal)
	signal[1] = protocol.OpcodeCardState
	signal[5] = byte(protocol.CardFeliCa)
	copy(signal[6:14], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	transport.reads <- signal

	deadline := time.Now().Add(time.Second)
	for session.CurrentCard().Type != protocol.CardFeliCa && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := protocol.FormatIDm(session.CurrentCard().IDm); got != "0102030405060708" {
		t.Fatalf("unexpected IDm %s", got)
	}

	none := make([]byte, protocol.ReportSize)
	none[0] = byte(protocol.MessageSignal)
	none[1] = protocol.OpcodeCardState
	transport.reads <- none
	for session.CurrentCard().Type != protocol.CardNone && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if session.CurrentCard().Type != protocol.CardNone {
		t.Fatal("card state was not cleared")
	}
}

func TestServiceReconnectsAfterReadLoopExits(t *testing.T) {
	devices := make(chan *fakeTransport, 2)
	open := func() (hid.Transport, hid.DeviceInfo, error) {
		select {
		case transport := <-devices:
			return transport, hid.DeviceInfo{Serial: "test"}, nil
		default:
			return nil, hid.DeviceInfo{}, hid.ErrDeviceNotFound
		}
	}

	first := newFakeTransport()
	devices <- first
	service := newService(open, time.Second, time.Millisecond)
	defer service.Close()
	respondToVersion(t, first)
	waitForConnection(t, service)

	_ = first.Close()
	deadline := time.Now().Add(time.Second)
	for service.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.Err() == nil {
		t.Fatal("service did not observe disconnection")
	}

	second := newFakeTransport()
	devices <- second
	respondToVersion(t, second)
	waitForConnection(t, service)
}

func respondToVersion(t *testing.T, transport *fakeTransport) {
	t.Helper()
	select {
	case request := <-transport.writes:
		header, err := protocol.DecodeHeader(request)
		if err != nil {
			t.Fatal(err)
		}
		response := make([]byte, protocol.ReportSize)
		response[0] = byte(protocol.MessageResponse)
		response[1] = protocol.OpcodeGetVersion
		binary.LittleEndian.PutUint16(response[2:4], header.Sequence)
		response[5] = 0
		response[6] = 1
		transport.reads <- response
	case <-time.After(time.Second):
		t.Fatal("version request was not written")
	}
}

func waitForConnection(t *testing.T, service *Service) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for service.Err() != nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.Err() != nil {
		t.Fatalf("service did not connect: %v", service.Err())
	}
}
