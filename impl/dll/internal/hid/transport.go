package hid

import "io"

type Transport interface {
	io.Closer
	Read() ([]byte, error)
	Write([]byte) error
}
