#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

#include "prhp.h"

typedef enum {
    USB_MESSAGE_HID,
    USB_MESSAGE_CDC,
} usb_message_type_t;

typedef struct {
    usb_message_type_t type;
    size_t size;
    uint8_t data[512];
} usb_message_t;

esp_err_t usb_device_init(void);
bool usb_device_receive(usb_message_t *message, uint32_t timeout_ms);
esp_err_t usb_device_send_prhp(const uint8_t report[PRHP_REPORT_SIZE]);
esp_err_t usb_device_send_cardio(uint8_t report_id, const uint8_t card_id[8]);
esp_err_t usb_device_send_cdc(const uint8_t *data, size_t size);
