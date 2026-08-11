#include "usb_device.h"

#include <string.h>

#include "class/hid/hid_device.h"
#include "esp_check.h"
#include "freertos/FreeRTOS.h"
#include "freertos/queue.h"
#include "freertos/task.h"
#include "tinyusb.h"
#include "tinyusb_cdc_acm.h"
#include "tinyusb_default_config.h"

enum {
    INTERFACE_CDC_CONTROL = 0,
    INTERFACE_CDC_DATA,
    INTERFACE_HID_PRHP,
    INTERFACE_HID_CARDIO,
    INTERFACE_COUNT,
};

enum {
    ENDPOINT_CDC_NOTIFICATION = 0x81,
    ENDPOINT_CDC_OUT = 0x02,
    ENDPOINT_CDC_IN = 0x82,
    ENDPOINT_PRHP_OUT = 0x03,
    ENDPOINT_PRHP_IN = 0x83,
    ENDPOINT_CARDIO_IN = 0x84,
};

#define CONFIGURATION_TOTAL_LENGTH \
    (TUD_CONFIG_DESC_LEN + TUD_CDC_DESC_LEN + TUD_HID_INOUT_DESC_LEN + \
     TUD_HID_DESC_LEN)

static QueueHandle_t messages;

static const tusb_desc_device_t device_descriptor = {
    .bLength = sizeof(tusb_desc_device_t),
    .bDescriptorType = TUSB_DESC_DEVICE,
    .bcdUSB = 0x0200,
    .bDeviceClass = TUSB_CLASS_MISC,
    .bDeviceSubClass = MISC_SUBCLASS_COMMON,
    .bDeviceProtocol = MISC_PROTOCOL_IAD,
    .bMaxPacketSize0 = CFG_TUD_ENDPOINT0_SIZE,
    .idVendor = 0x5041,
    .idProduct = 0x5245,
    .bcdDevice = 0x0100,
    .iManufacturer = 0x01,
    .iProduct = 0x02,
    .iSerialNumber = 0x03,
    .bNumConfigurations = 0x01,
};

static const uint8_t prhp_report_descriptor[] = {
    0x06, 0x50, 0xFF,       // Usage Page (Vendor 0xFF50)
    0x09, 0x01,             // Usage (0x01)
    0xA1, 0x01,             // Collection (Application)
    0x15, 0x00,             // Logical Minimum (0)
    0x26, 0xFF, 0x00,       // Logical Maximum (255)
    0x75, 0x08,             // Report Size (8)
    0x95, PRHP_REPORT_SIZE, // Report Count (64)
    0x09, 0x01,             // Usage (0x01)
    0x81, 0x02,             // Input (Data, Variable, Absolute)
    0x95, PRHP_REPORT_SIZE, // Report Count (64)
    0x09, 0x01,             // Usage (0x01)
    0x91, 0x02,             // Output (Data, Variable, Absolute)
    0xC0,                   // End Collection
};

static const uint8_t cardio_report_descriptor[] = {
    0x06, 0xCA, 0xFF, // Usage Page (Vendor 0xFFCA)
    0x09, 0x01,       // Usage (0x01)
    0xA1, 0x01,       // Collection (Application)

    0x85, 0x01,       // Report ID (1)
    0x09, 0x41,       // Usage (0x41, MIFARE)
    0x15, 0x00,       // Logical Minimum (0)
    0x26, 0xFF, 0x00, // Logical Maximum (255)
    0x75, 0x08,       // Report Size (8)
    0x95, 0x08,       // Report Count (8)
    0x81, 0x02,       // Input (Data, Variable, Absolute)

    0x85, 0x02,       // Report ID (2)
    0x09, 0x42,       // Usage (0x42, FeliCa)
    0x15, 0x00,       // Logical Minimum (0)
    0x26, 0xFF, 0x00, // Logical Maximum (255)
    0x75, 0x08,       // Report Size (8)
    0x95, 0x08,       // Report Count (8)
    0x81, 0x02,       // Input (Data, Variable, Absolute)

    0xC0,             // End Collection
};

static const char *string_descriptor[] = {
    (char[]) {0x09, 0x04},
    "PA Reader",
    "PA Reader NFC",
    "000001",
    "SEGA Reader",
    "PA Reader PRHP",
    "PA Reader CardIO",
};

static const uint8_t configuration_descriptor[] = {
    TUD_CONFIG_DESCRIPTOR(
            1,
            INTERFACE_COUNT,
            0,
            CONFIGURATION_TOTAL_LENGTH,
            TUSB_DESC_CONFIG_ATT_REMOTE_WAKEUP,
            100),

    TUD_CDC_DESCRIPTOR(
            INTERFACE_CDC_CONTROL,
            4,
            ENDPOINT_CDC_NOTIFICATION,
            8,
            ENDPOINT_CDC_OUT,
            ENDPOINT_CDC_IN,
            64),

    TUD_HID_INOUT_DESCRIPTOR(
            INTERFACE_HID_PRHP,
            5,
            HID_ITF_PROTOCOL_NONE,
            sizeof(prhp_report_descriptor),
            ENDPOINT_PRHP_OUT,
            ENDPOINT_PRHP_IN,
            PRHP_REPORT_SIZE,
            1),

    TUD_HID_DESCRIPTOR(
            INTERFACE_HID_CARDIO,
            6,
            HID_ITF_PROTOCOL_NONE,
            sizeof(cardio_report_descriptor),
            ENDPOINT_CARDIO_IN,
            9,
            1),
};

uint8_t const *tud_hid_descriptor_report_cb(uint8_t instance)
{
    return instance == 0 ? prhp_report_descriptor : cardio_report_descriptor;
}

uint16_t tud_hid_get_report_cb(
        uint8_t instance,
        uint8_t report_id,
        hid_report_type_t report_type,
        uint8_t *buffer,
        uint16_t requested_length)
{
    (void) instance;
    (void) report_id;
    (void) report_type;
    (void) buffer;
    (void) requested_length;
    return 0;
}

void tud_hid_set_report_cb(
        uint8_t instance,
        uint8_t report_id,
        hid_report_type_t report_type,
        const uint8_t *buffer,
        uint16_t size)
{
    (void) report_type;
    if (instance != 0 || report_id != 0 || size != PRHP_REPORT_SIZE) {
        return;
    }

    usb_message_t message = {
        .type = USB_MESSAGE_HID,
        .size = PRHP_REPORT_SIZE,
    };
    memcpy(message.data, buffer, PRHP_REPORT_SIZE);
    xQueueSend(messages, &message, 0);
}

static void cdc_receive_callback(int interface, cdcacm_event_t *event)
{
    (void) event;
    usb_message_t message = {.type = USB_MESSAGE_CDC};
    if (tinyusb_cdcacm_read(
                interface,
                message.data,
                sizeof(message.data),
                &message.size) == ESP_OK &&
            message.size > 0) {
        xQueueSend(messages, &message, 0);
    }
}

esp_err_t usb_device_init(void)
{
    messages = xQueueCreate(16, sizeof(usb_message_t));
    if (messages == NULL) {
        return ESP_ERR_NO_MEM;
    }

    tinyusb_config_t config = TINYUSB_DEFAULT_CONFIG();
    config.descriptor.device = &device_descriptor;
    config.descriptor.full_speed_config = configuration_descriptor;
    config.descriptor.string = string_descriptor;
    config.descriptor.string_count = sizeof(string_descriptor) / sizeof(string_descriptor[0]);
#if TUD_OPT_HIGH_SPEED
    config.descriptor.high_speed_config = configuration_descriptor;
#endif
    ESP_RETURN_ON_ERROR(tinyusb_driver_install(&config), "usb", "TinyUSB install failed");

    const tinyusb_config_cdcacm_t cdc = {
        .cdc_port = TINYUSB_CDC_ACM_0,
        .callback_rx = cdc_receive_callback,
        .callback_rx_wanted_char = NULL,
        .callback_line_state_changed = NULL,
        .callback_line_coding_changed = NULL,
    };
    return tinyusb_cdcacm_init(&cdc);
}

bool usb_device_receive(usb_message_t *message, uint32_t timeout_ms)
{
    return xQueueReceive(messages, message, pdMS_TO_TICKS(timeout_ms)) == pdTRUE;
}

static esp_err_t send_hid(
        uint8_t instance,
        uint8_t report_id,
        const uint8_t *report,
        size_t size)
{
    for (int attempts = 0; attempts < 100; ++attempts) {
        if (!tud_mounted()) {
            return ESP_ERR_INVALID_STATE;
        }
        if (tud_hid_n_ready(instance)) {
            return tud_hid_n_report(instance, report_id, report, size)
                    ? ESP_OK
                    : ESP_FAIL;
        }
        vTaskDelay(pdMS_TO_TICKS(1));
    }
    return ESP_ERR_TIMEOUT;
}

esp_err_t usb_device_send_prhp(const uint8_t report[PRHP_REPORT_SIZE])
{
    return send_hid(0, 0, report, PRHP_REPORT_SIZE);
}

esp_err_t usb_device_send_cardio(uint8_t report_id, const uint8_t card_id[8])
{
    return send_hid(1, report_id, card_id, 8);
}

esp_err_t usb_device_send_cdc(const uint8_t *data, size_t size)
{
    ESP_RETURN_ON_ERROR(
            tinyusb_cdcacm_write_queue(TINYUSB_CDC_ACM_0, data, size),
            "usb",
            "CDC queue failed");
    return tinyusb_cdcacm_write_flush(TINYUSB_CDC_ACM_0, 0);
}
