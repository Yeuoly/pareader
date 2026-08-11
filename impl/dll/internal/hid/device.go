package hid

import (
	"bytes"
	"errors"
	"fmt"
	"sync/atomic"

	khid "github.com/sstallion/go-hid"

	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

const (
	DefaultUsagePage uint16 = 0xFF50
	DefaultUsage     uint16 = 0x0001
)

var (
	ErrDeviceNotFound = errors.New("PA Reader HID device not found")
	ErrShortReport    = errors.New("short HID report")
	ErrClosed         = errors.New("PA Reader HID device is closed")
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
		return nil, DeviceInfo{}, fmt.Errorf("enumerate PA Reader HID: %w", err)
	}
	if selected == nil {
		return nil, DeviceInfo{}, ErrDeviceNotFound
	}

	device, err := khid.OpenPath(selected.Path)
	if err != nil {
		return nil, DeviceInfo{}, fmt.Errorf("open PA Reader HID: %w", err)
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
		return nil, fmt.Errorf("read HID report: %w", err)
	}
	if n != protocol.ReportSize {
		return nil, fmt.Errorf("%w: got %d bytes, want %d", ErrShortReport, n, protocol.ReportSize)
	}
	return raw, nil
}

func (d *Device) Write(raw []byte) error {
	if len(raw) != protocol.ReportSize {
		return fmt.Errorf("%w: got %d bytes, want %d", protocol.ErrInvalidReport, len(raw), protocol.ReportSize)
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
				d.closeTransport()
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
		return fmt.Errorf("write HID report: %w", err)
	}
	if n != len(report) {
		return fmt.Errorf("%w: wrote %d bytes, want %d", ErrShortReport, n, len(report))
	}
	return nil
}

func (d *Device) Close() error {
	d.closeTransport()
	<-d.writerDone
	return d.device.Close()
}

func (d *Device) closeTransport() {
	if d.closed.CompareAndSwap(false, true) {
		close(d.done)
	}
}
