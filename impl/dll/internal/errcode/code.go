package errcode

// Code is a stable, printable error identifier.
type Code string

func (code Code) Error() string {
	return string(code)
}
