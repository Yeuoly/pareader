# PA Reader Firmware

This ESP-IDF firmware exposes one ESP32-S3 composite USB device with three
independent functions:

- CDC-ACM for the SEGA reader protocol;
- USB interface 2 / TinyUSB HID instance 0 for PRHP (`Usage Page 0xFF50`);
- USB interface 3 / TinyUSB HID instance 1 for CardIO (`Usage Page 0xFFCA`).

Each HID interface owns one top-level application collection and its own
interrupt IN endpoint. PRHP also has an interrupt OUT endpoint. The PRHP and
CardIO collections are not combined into one HID interface.

## PN532 wiring

Set the Elechouse NFC Module V3 to I2C mode before power-up:

| Switch | Position |
|---|---|
| `1` / `HSU0` | `ON` (`1`) |
| `2` / `HSU1` | `OFF` (`0`) |

| NFC Module V3 | ESP32-S3 | Configuration |
|---|---:|---|
| `SDA` | GPIO 2 | `PAREADER_PN532_SDA_GPIO` |
| `SCL` | GPIO 3 | `PAREADER_PN532_SCL_GPIO` |
| `GND` | GND | Common ground |
| `VCC` | 3V3 | Module supply |

The I2C pins are configurable with `idf.py menuconfig` under **PA Reader**.
Connect the ESP32-S3 native USB port rather than a UART-only bridge.

## Runtime

TinyUSB callbacks copy PRHP Output reports and CDC input into one FreeRTOS
queue. The main task services that queue and performs an NFC observation about
every 500 ms.

The task stores only one card value:

```c
reader_card_t current = {0};

reader_card_t observed;
if (reader_read_card(&observed) == ESP_OK &&
        memcmp(&current, &observed, sizeof(current)) != 0) {
    current = observed;
    prhp_publish_card_state(&current);
    cardio_publish_card_state(&current);
}
```

The equality check gives both HID outputs the same transition behavior. A card
is published once, a held card is silent, removal publishes one empty state,
and later empty observations are silent.

PRHP encodes the observation as a 64-byte `CARD_STATE` signal. CardIO encodes
the same observation as Report ID 1 for MIFARE or Report ID 2 for FeliCa. An
empty CardIO state is an all-zero report. Neither output requires a Host card
request.

The CDC path remains command-driven and implements SEGA framing and NFC
commands independently of both HID protocols. SEGA and PRHP LED commands are
accepted for interface compatibility and intentionally have no hardware effect.

## Build and flash

With ESP-IDF 5.5 available:

```sh
idf.py set-target esp32s3
idf.py build
idf.py -p PORT flash monitor
```

The TinyUSB configuration must keep `CONFIG_TINYUSB_HID_COUNT=2`.
