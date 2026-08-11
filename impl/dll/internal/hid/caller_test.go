package hid

import (
	"errors"
	"testing"
	"time"
)

func TestCallerRoutesResponse(t *testing.T) {
	caller := NewCaller()
	sequence, response, err := caller.Allocate(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	want := []byte{1, 2, 3}
	if err := caller.Consume(sequence, want); err != nil {
		t.Fatal(err)
	}
	got := <-response
	if string(got) != string(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if err := caller.Consume(sequence, want); !errors.Is(err, ErrUnknownSequence) {
		t.Fatalf("duplicate consume error = %v", err)
	}
}

func TestCallerExpiresLazily(t *testing.T) {
	caller := NewCaller()
	sequence, _, err := caller.Allocate(time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := caller.Consume(sequence, []byte{1}); !errors.Is(err, ErrUnknownSequence) {
		t.Fatalf("expired consume error = %v", err)
	}
}
