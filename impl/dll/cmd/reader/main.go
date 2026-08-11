package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Yeuoly/pareader/impl/dll/internal/aimeio"
	"github.com/Yeuoly/pareader/impl/dll/internal/config"
	pareaderhid "github.com/Yeuoly/pareader/impl/dll/internal/hid"
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
		return fmt.Errorf("invalid -vid: %w", err)
	}
	pid, err := config.ParseUint16(*pidText)
	if err != nil {
		return fmt.Errorf("invalid -pid: %w", err)
	}

	device, info, err := pareaderhid.Open(pareaderhid.DeviceConfig{
		VendorID:  vid,
		ProductID: pid,
		UsagePage: pareaderhid.DefaultUsagePage,
		Usage:     pareaderhid.DefaultUsage,
		Serial:    *serial,
	})
	if err != nil {
		return err
	}

	service := aimeio.NewService(device, *timeout)
	defer service.Close()

	version, err := service.Version(context.Background())
	if err != nil {
		return fmt.Errorf("query protocol version: %w", err)
	}
	if version.Major != 0 || version.Minor != 1 {
		return fmt.Errorf("unsupported PRHP version %d.%d", version.Major, version.Minor)
	}

	model := newModel(service, info, version, *interval)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}
