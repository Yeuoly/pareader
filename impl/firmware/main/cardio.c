#include "cardio.h"

#include <string.h>

#include "usb_device.h"

enum {
    CARDIO_REPORT_MIFARE = 0x01,
    CARDIO_REPORT_FELICA = 0x02,
};

void cardio_publish_card_state(const reader_card_t *current)
{
    uint8_t card_id[8] = {0};
    uint8_t report_id;

    if (current->type == READER_CARD_FELICA) {
        report_id = CARDIO_REPORT_FELICA;
        memcpy(card_id, current->idm, sizeof(current->idm));
    } else if (current->type == READER_CARD_MIFARE) {
        report_id = CARDIO_REPORT_MIFARE;
        card_id[0] = 0xE0;
        card_id[1] = 0x04;
        memcpy(&card_id[2], current->mifare_uid, sizeof(current->mifare_uid));
        card_id[6] = current->mifare_uid[0];
        card_id[7] = current->mifare_uid[1];
    } else {
        report_id = 0;
    }

    usb_device_send_cardio(report_id, card_id);
}
