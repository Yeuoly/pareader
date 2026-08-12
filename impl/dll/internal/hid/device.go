package hid

import (
	"bytes"
	"sync/atomic"

	khid "github.com/sstallion/go-hid"

	"github.com/Yeuoly/pareader/impl/dll/internal/errcode"
	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

const (
	DefaultUsagePage uint16 = 0xFF50
	DefaultUsage     uint16 = 0x0001
)

const (
	ErrEnumerate      errcode.Code = "E0201"
	ErrDeviceNotFound errcode.Code = "E0202"
	ErrOpen           errcode.Code = "E0203"
	ErrShortReport    errcode.Code = "E0204"
	ErrClosed         errcode.Code = "E0205"
	ErrRead           errcode.Code = "E0206"
	ErrWrite          errcode.Code = "E0207"
	ErrClose          errcode.Code = "E0208"
)

type DeviceConfig struct {
	VendorID  uint16
	ProductID uint16
	UsagePage uint16
	Usage     uint16
	Serial    string
}

type DeviceInfo struct {
	Path         string
	VendorID     uint16
	ProductID    uint16
	Serial       string
	Manufacturer string
	Product      string
}

type Device struct {
	device     *khid.Device
	writes     chan []byte
	done       chan struct{}
	writerDone chan struct{}
	stopping   atomic.Bool
	closed     atomic.Bool
}

func Open(config DeviceConfig) (*Device, DeviceInfo, error) {
	if config.UsagePage == 0 {
		config.UsagePage = DefaultUsagePage
	}
	if config.Usage == 0 {
		config.Usage = DefaultUsage
	}

	configureOpenMode()

	var selected *khid.DeviceInfo
	err := khid.Enumerate(config.VendorID, config.ProductID, func(candidate *khid.DeviceInfo) error {
		if candidate.UsagePage != config.UsagePage || candidate.Usage != config.Usage {
			return nil
		}
		if config.Serial != "" && candidate.SerialNbr != config.Serial {
			return nil
		}
		copy := *candidate
		selected = &copy
		return nil
	})
	if err != nil {
		return nil, DeviceInfo{}, ErrEnumerate
	}
	if selected == nil {
		return nil, DeviceInfo{}, ErrDeviceNotFound
	}

	device, err := khid.OpenPath(selected.Path)
	if err != nil {
		return nil, DeviceInfo{}, ErrOpen
	}
	transport := &Device{
		device:     device,
		writes:     make(chan []byte, 64),
		done:       make(chan struct{}),
		writerDone: make(chan struct{}),
	}
	go transport.writeLoop()

	return transport, DeviceInfo{
		Path:         selected.Path,
		VendorID:     selected.VendorID,
		ProductID:    selected.ProductID,
		Serial:       selected.SerialNbr,
		Manufacturer: selected.MfrStr,
		Product:      selected.ProductStr,
	}, nil
}

func (d *Device) Read() ([]byte, error) {
	raw := make([]byte, protocol.ReportSize)
	n, err := d.device.Read(raw)
	if err != nil {
		d.stop()
		return nil, ErrRead
	}
	if n != protocol.ReportSize {
		d.stop()
		return nil, ErrShortReport
	}
	return raw, nil
}

func (d *Device) Write(raw []byte) error {
	if len(raw) != protocol.ReportSize {
		return protocol.ErrInvalidReport
	}
	if d.stopping.Load() {
		return ErrClosed
	}
	select {
	case d.writes <- bytes.Clone(raw):
		return nil
	case <-d.done:
		return ErrClosed
	}
}

func (d *Device) writeLoop() {
	defer close(d.writerDone)
	for {
		select {
		case raw := <-d.writes:
			if err := d.writeReport(raw); err != nil {
				d.stop()
				_ = d.closeHandle()
				return
			}
		case <-d.done:
			return
		}
	}
}

func (d *Device) writeReport(raw []byte) error {

	// hidapi includes the Report ID as the first write byte. PRHP itself does
	// not use Report IDs, so the API-only prefix is zero.
	report := make([]byte, protocol.ReportSize+1)
	copy(report[1:], raw)

	n, err := d.device.Write(report)
	if err != nil {
		return ErrWrite
	}
	if n != len(report) {
		return ErrShortReport
	}
	return nil
}

func (d *Device) Close() error {
	d.stop()
	<-d.writerDone
	return d.closeHandle()
}

func (d *Device) closeHandle() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	if err := d.device.Close(); err != nil {
		return ErrClose
	}
	return nil
}

func (d *Device) stop() {
	if d.stopping.CompareAndSwap(false, true) {
		close(d.done)
	}
}
