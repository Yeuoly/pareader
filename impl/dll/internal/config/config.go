package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Yeuoly/pareader/impl/dll/internal/hid"
)

const (
	DefaultVendorID  uint16 = 0x5041
	DefaultProductID uint16 = 0x5245
)

type Config struct {
	Device  hid.DeviceConfig
	Timeout time.Duration
}

func FromEnvironment() (Config, error) {
	vendorID, err := parseUint16(environment("PAREADER_VID", "5041"))
	if err != nil {
		return Config{}, fmt.Errorf("PAREADER_VID: %w", err)
	}
	productID, err := parseUint16(environment("PAREADER_PID", "5245"))
	if err != nil {
		return Config{}, fmt.Errorf("PAREADER_PID: %w", err)
	}
	timeout, err := time.ParseDuration(environment("PAREADER_TIMEOUT", "1s"))
	if err != nil {
		return Config{}, fmt.Errorf("PAREADER_TIMEOUT: %w", err)
	}

	return Config{
		Device: hid.DeviceConfig{
			VendorID:  vendorID,
			ProductID: productID,
			UsagePage: hid.DefaultUsagePage,
			Usage:     hid.DefaultUsage,
			Serial:    os.Getenv("PAREADER_SERIAL"),
		},
		Timeout: timeout,
	}, nil
}

func ParseUint16(value string) (uint16, error) {
	return parseUint16(value)
}

func parseUint16(value string) (uint16, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	parsed, err := strconv.ParseUint(value, 16, 16)
	if err != nil {
		return 0, err
	}
	return uint16(parsed), nil
}

func environment(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
