package main

import (
	"strings"
	"testing"

	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

func TestViewShowsCardIdentifier(t *testing.T) {
	m := model{
		version: protocol.Version{Major: 0, Minor: 1},
		card: protocol.Card{
			Type: protocol.CardFeliCa,
			IDm:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		},
	}
	if view := m.View(); !strings.Contains(view, "0102030405060708") {
		t.Fatalf("view does not contain card identifier: %q", view)
	}
}
