#include <string.h>

#include "esp_check.h"
#include "esp_timer.h"

#include "cardio.h"
#include "prhp.h"
#include "reader.h"
#include "sega.h"
#include "usb_device.h"

#define CARD_SCAN_INTERVAL_US 200000

void app_main(void)
{
    ESP_ERROR_CHECK(reader_init());
    sega_init();
    ESP_ERROR_CHECK(usb_device_init());

    reader_card_t current = {0};
    int64_t next_scan = 0;
    usb_message_t message;
    for (;;) {
        if (usb_device_receive(&message, 50)) {
            if (message.type == USB_MESSAGE_HID && message.size == PRHP_REPORT_SIZE) {
                prhp_dispatch(message.data);
            } else if (message.type == USB_MESSAGE_CDC) {
                sega_receive(message.data, message.size);
            }
        }

        const int64_t now = esp_timer_get_time();
        if (now < next_scan) {
            continue;
        }
        next_scan = now + CARD_SCAN_INTERVAL_US;

        reader_card_t observed;
        if (reader_read_card(&observed) != ESP_OK ||
                memcmp(&current, &observed, sizeof(current)) == 0) {
            continue;
        }

        current = observed;
        prhp_publish_card_state(&current);
        cardio_publish_card_state(&current);
    }
}
