package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Yeuoly/pareader/impl/dll/internal/aimeio"
	"github.com/Yeuoly/pareader/impl/dll/internal/config"
	pareaderhid "github.com/Yeuoly/pareader/impl/dll/internal/hid"
	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pareader:", err)
		os.Exit(1)
	}
}

func run() error {
	vidText := flag.String("vid", "5041", "USB vendor ID in hexadecimal")
	pidText := flag.String("pid", "5245", "USB product ID in hexadecimal")
	serial := flag.String("serial", "", "optional USB serial number")
	timeout := flag.Duration("timeout", time.Second, "timeout for request-response commands")
	interval := flag.Duration("interval", 100*time.Millisecond, "card-state display refresh interval")
	flag.Parse()

	vid, err := config.ParseUint16(*vidText)
	if err != nil {
		return config.ErrInvalidVendorID
	}
	pid, err := config.ParseUint16(*pidText)
	if err != nil {
		return config.ErrInvalidProductID
	}

	deviceConfig := pareaderhid.DeviceConfig{
		VendorID:  vid,
		ProductID: pid,
		UsagePage: pareaderhid.DefaultUsagePage,
		Usage:     pareaderhid.DefaultUsage,
		Serial:    *serial,
	}
	service := aimeio.NewService(deviceConfig, *timeout)
	defer service.Close()

	model := newModel(service, pareaderhid.DeviceInfo{
		VendorID:  vid,
		ProductID: pid,
		Product:   "PA Reader",
	}, protocol.Version{Major: 0, Minor: 1}, *interval)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}
