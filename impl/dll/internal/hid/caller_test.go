package hid

import (
	"errors"
	"sync"
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

func TestCallerConsumesResponseOnce(t *testing.T) {
	caller := NewCaller()
	sequence, response, err := caller.Allocate(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- caller.Consume(sequence, []byte{1})
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	var delivered, rejected int
	for err := range results {
		switch {
		case err == nil:
			delivered++
		case errors.Is(err, ErrUnknownSequence):
			rejected++
		default:
			t.Fatalf("unexpected consume error: %v", err)
		}
	}
	if delivered != 1 || rejected != 1 {
		t.Fatalf("delivered %d responses and rejected %d", delivered, rejected)
	}
	if got := <-response; len(got) != 1 || got[0] != 1 {
		t.Fatalf("response = %v", got)
	}
}
