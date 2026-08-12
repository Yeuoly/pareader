//go:build windows

package main

/*
#include <stdint.h>
#include <stddef.h>

typedef int32_t HRESULT;
*/
import "C"

import (
	"sync/atomic"
	"unsafe"

	"github.com/Yeuoly/pareader/impl/dll/internal/aimeio"
	"github.com/Yeuoly/pareader/impl/dll/internal/config"
)

const (
	sOK         uint32 = 0x00000000
	sFalse      uint32 = 0x00000001
	eFail       uint32 = 0x80004005
	eInvalidArg uint32 = 0x80070057
)

var activeService atomic.Pointer[aimeio.Service]

func result(value uint32) C.HRESULT {
	return C.HRESULT(int32(value))
}

//export aime_io_get_api_version
func aime_io_get_api_version() C.uint16_t {
	// AimeIO 1.0 plus the optional get_mifare_block extension.
	return 0x0100
}

//export aime_io_init
func aime_io_init() C.HRESULT {
	if activeService.Load() != nil {
		return result(sOK)
	}

	settings, err := config.FromEnvironment()
	if err != nil {
		return result(eFail)
	}
	service := aimeio.NewService(settings.Device, settings.Timeout)

	if !activeService.CompareAndSwap(nil, service) {
		_ = service.Close()
	}
	return result(sOK)
}

//export aime_io_nfc_poll
func aime_io_nfc_poll(unit C.uint8_t) C.HRESULT {
	service := activeService.Load()
	if service == nil {
		return result(eFail)
	}
	if unit != 0 {
		return result(sFalse)
	}
	return result(sOK)
}

//export aime_io_nfc_get_aime_id
func aime_io_nfc_get_aime_id(
	unit C.uint8_t,
	luid *C.uint8_t,
	luidSize C.size_t,
) C.HRESULT {
	service := activeService.Load()
	if service == nil {
		return result(eFail)
	}
	if unit != 0 {
		return result(sFalse)
	}
	if luid == nil || luidSize != 10 {
		return result(eInvalidArg)
	}

	id, present := service.AimeID()
	if !present {
		return result(sFalse)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(luid)), 10), id[:])
	return result(sOK)
}

//export aime_io_nfc_get_mifare_block
func aime_io_nfc_get_mifare_block(
	unit C.uint8_t,
	blocks *C.uint8_t,
	blocksSize C.size_t,
) C.HRESULT {
	service := activeService.Load()
	if service == nil {
		return result(eFail)
	}
	if unit != 0 {
		return result(sFalse)
	}
	if blocks == nil || blocksSize != 32 {
		return result(eInvalidArg)
	}

	data, present := service.MIFAREBlocks()
	if !present {
		return result(sFalse)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(blocks)), 32), data[:])
	return result(sOK)
}

//export aime_io_nfc_get_felica_id
func aime_io_nfc_get_felica_id(unit C.uint8_t, idm *C.uint64_t) C.HRESULT {
	service := activeService.Load()
	if service == nil {
		return result(eFail)
	}
	if unit != 0 {
		return result(sFalse)
	}
	if idm == nil {
		return result(eInvalidArg)
	}

	value, present := service.FeliCaID()
	if !present {
		return result(sFalse)
	}
	*idm = C.uint64_t(value)
	return result(sOK)
}

//export aime_io_led_set_color
func aime_io_led_set_color(unit, red, green, blue C.uint8_t) {
	service := activeService.Load()
	if service == nil || unit != 0 {
		return
	}
	_ = service.SetLED(byte(red), byte(green), byte(blue))
}
