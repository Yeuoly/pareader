//go:build darwin

package hid

import khid "github.com/sstallion/go-hid"

func configureOpenMode() {
	khid.SetOpenExclusive(false)
}
