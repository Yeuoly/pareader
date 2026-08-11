package hid

import (
	"errors"
	"testing"
	"time"

	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

func TestRouterUsesExplicitMessageType(t *testing.T) {
	caller := NewCaller()
	sequence, response, err := caller.Allocate(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(caller)
	request := protocol.NewRequest(protocol.OpcodeGetVersion, sequence)
	if err := router.Dispatch(request); !errors.Is(err, protocol.ErrUnexpectedType) {
		t.Fatalf("request dispatch error = %v", err)
	}

	want := []byte{0xAA}
	if err := caller.Consume(sequence, want); err != nil {
		t.Fatalf("request was incorrectly consumed as response: %v", err)
	}
	if got := <-response; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("unexpected response % X", got)
	}
}

func TestRouterDispatchesSignalByType(t *testing.T) {
	router := NewRouter(NewCaller())
	called := false
	router.Handle(protocol.OpcodeSetLED, func([]byte) error {
		called = true
		return nil
	})

	if err := router.Dispatch(protocol.EncodeSetLED(1, 2, 3)); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("signal handler was not called")
	}
}
