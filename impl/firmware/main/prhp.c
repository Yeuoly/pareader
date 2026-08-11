#include "prhp.h"

#include <string.h>

#include "reader.h"
#include "usb_device.h"

enum {
	PRHP_REQUEST = 0x01,
	PRHP_RESPONSE = 0x02,
	PRHP_SIGNAL = 0x03,
};

enum {
    PRHP_GET_VERSION = 0x01,
    PRHP_CARD_STATE = 0x02,
    PRHP_SET_LED = 0x03,
};

enum {
    PRHP_SUCCESS = 0x00,
    PRHP_UNKNOWN_OPCODE = 0x01,
    PRHP_INVALID_MESSAGE = 0x02,
};

static uint16_t read_le16(const uint8_t *bytes)
{
    return (uint16_t) bytes[0] | ((uint16_t) bytes[1] << 8);
}

static void write_le16(uint8_t *bytes, uint16_t value)
{
    bytes[0] = (uint8_t) value;
    bytes[1] = (uint8_t) (value >> 8);
}

static void begin_response(
        uint8_t response[PRHP_REPORT_SIZE],
        uint8_t opcode,
        uint8_t status,
        uint16_t sequence)
{
	memset(response, 0, PRHP_REPORT_SIZE);
	response[0] = PRHP_RESPONSE;
	response[1] = opcode;
	write_le16(&response[2], sequence);
	response[4] = status;
}

static void send_error(uint8_t opcode, uint8_t status, uint16_t sequence)
{
    uint8_t response[PRHP_REPORT_SIZE];
    begin_response(response, opcode, status, sequence);
    usb_device_send_prhp(response);
}

void prhp_publish_card_state(const reader_card_t *card)
{
    uint8_t signal[PRHP_REPORT_SIZE] = {0};
    signal[0] = PRHP_SIGNAL;
    signal[1] = PRHP_CARD_STATE;

    if (card->type == READER_CARD_MIFARE && !card->has_blocks) {
        signal[5] = READER_CARD_NONE;
    } else {
        signal[5] = (uint8_t) card->type;
    }

    if (signal[5] == READER_CARD_MIFARE) {
        memcpy(&signal[6], card->luid, sizeof(card->luid));
        memcpy(&signal[16], card->blocks, sizeof(card->blocks));
    } else if (signal[5] == READER_CARD_FELICA) {
        memcpy(&signal[6], card->idm, sizeof(card->idm));
    }
    usb_device_send_prhp(signal);
}

void prhp_dispatch(const uint8_t request[PRHP_REPORT_SIZE])
{
	const uint8_t message_type = request[0];
	const uint8_t opcode = request[1];
	const uint16_t sequence = read_le16(&request[2]);

	if (request[4] != 0) {
		if (message_type == PRHP_REQUEST && sequence != 0) {
			send_error(opcode, PRHP_INVALID_MESSAGE, sequence);
		}
		return;
	}

	if (message_type == PRHP_SIGNAL) {
		if (sequence == 0 && opcode == PRHP_SET_LED) {
			reader_set_led(request[5], request[6], request[7]);
		}
		return;
	}

	if (message_type != PRHP_REQUEST || sequence == 0) {
		return;
	}

    switch (opcode) {
    case PRHP_GET_VERSION: {
		uint8_t response[PRHP_REPORT_SIZE];
		begin_response(response, opcode, PRHP_SUCCESS, sequence);
		response[5] = 0;
		response[6] = 1;
        usb_device_send_prhp(response);
        return;
    }

	default:
		send_error(opcode, PRHP_UNKNOWN_OPCODE, sequence);
		return;
    }
}
