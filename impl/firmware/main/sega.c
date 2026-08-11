#include "sega.h"

#include <stdbool.h>
#include <string.h>

#include "reader.h"
#include "usb_device.h"

#define SG_SYNC 0xE0
#define SG_ESCAPE 0xD0

#define SG_ADDRESS_NFC 0x00
#define SG_ADDRESS_LED 0x08

enum {
    SG_NFC_GET_FW_VERSION = 0x30,
    SG_NFC_GET_HW_VERSION = 0x32,
    SG_NFC_RADIO_ON = 0x40,
    SG_NFC_RADIO_OFF = 0x41,
    SG_NFC_POLL = 0x42,
    SG_NFC_MIFARE_SELECT_TAG = 0x43,
    SG_NFC_MIFARE_SET_KEY_AIME = 0x50,
    SG_NFC_MIFARE_AUTHENTICATE_AIME = 0x51,
    SG_NFC_MIFARE_READ_BLOCK = 0x52,
    SG_NFC_MIFARE_SET_KEY_BANA = 0x54,
    SG_NFC_MIFARE_AUTHENTICATE_BANA = 0x55,
    SG_NFC_TO_UPDATE_MODE = 0x60,
    SG_NFC_SEND_HEX_DATA = 0x61,
    SG_NFC_RESET = 0x62,
    SG_NFC_FELICA_ENCAP = 0x71,
};

enum {
    SG_LED_SET_COLOR = 0x81,
    SG_LED_GET_INFO = 0xF0,
    SG_LED_RESET = 0xF5,
};

typedef struct {
    bool synchronized;
    bool escaped;
    uint8_t bytes[257];
    size_t size;
} sg_parser_t;

static sg_parser_t parser;

static void encode_byte(uint8_t byte, uint8_t *output, size_t *size)
{
    if (byte == SG_ESCAPE || byte == SG_SYNC) {
        output[(*size)++] = SG_ESCAPE;
        output[(*size)++] = byte - 1;
    } else {
        output[(*size)++] = byte;
    }
}

static void send_response(
        const uint8_t *request,
        uint8_t status,
        const uint8_t *payload,
        size_t payload_size)
{
    if (payload_size > 249) {
        return;
    }

    uint8_t decoded[256] = {0};
    const size_t decoded_size = 6 + payload_size;
    decoded[0] = (uint8_t) decoded_size;
    decoded[1] = request[1];
    decoded[2] = request[2];
    decoded[3] = request[3];
    decoded[4] = status;
    decoded[5] = (uint8_t) payload_size;
    if (payload_size > 0) {
        memcpy(&decoded[6], payload, payload_size);
    }

    uint8_t encoded[520] = {SG_SYNC};
    size_t encoded_size = 1;
    uint8_t checksum = 0;
    for (size_t i = 0; i < decoded_size; ++i) {
        checksum += decoded[i];
        encode_byte(decoded[i], encoded, &encoded_size);
    }
    encode_byte(checksum, encoded, &encoded_size);
    usb_device_send_cdc(encoded, encoded_size);
}

static void dispatch_led(const uint8_t *request, const uint8_t *payload, size_t payload_size)
{
    switch (request[3]) {
    case SG_LED_SET_COLOR:
        if (payload_size == 3) {
            reader_set_led(payload[0], payload[1], payload[2]);
        }
        return;
    case SG_LED_RESET: {
        const uint8_t response[] = {0};
        reader_set_led(0, 0, 0);
        send_response(request, 0, response, sizeof(response));
        return;
    }
    case SG_LED_GET_INFO: {
        static const uint8_t version[] = {
            '0', '0', '0', '-', '0', '0', '0', '0', '0', 0xFF, 0x11, 0x40,
        };
        send_response(request, 0, version, sizeof(version));
        return;
    }
    default:
        send_response(request, 1, NULL, 0);
        return;
    }
}

static void dispatch_nfc(const uint8_t *request, const uint8_t *payload, size_t payload_size)
{
    esp_err_t err = ESP_OK;
    switch (request[3]) {
    case SG_NFC_RESET:
        send_response(request, 3, NULL, 0);
        return;

    case SG_NFC_GET_FW_VERSION: {
        const uint8_t version[] = {0x94};
        send_response(request, 0, version, sizeof(version));
        return;
    }

    case SG_NFC_GET_HW_VERSION: {
        static const uint8_t version[] = {'8', '3', '7', '-', '1', '5', '2', '8', '6'};
        send_response(request, 0, version, sizeof(version));
        return;
    }

    case SG_NFC_RADIO_ON:
        err = reader_radio_set(true);
        break;
    case SG_NFC_RADIO_OFF:
        err = reader_radio_set(false);
        break;

    case SG_NFC_POLL: {
        reader_card_t card;
        err = reader_read_card(&card);
        if (err != ESP_OK) {
            break;
        }

        uint8_t response[19] = {0};
        if (card.type == READER_CARD_MIFARE) {
            response[0] = 1;
            response[1] = 0x10;
            response[2] = 4;
            memcpy(&response[3], card.mifare_uid, 4);
            send_response(request, 0, response, 7);
        } else if (card.type == READER_CARD_FELICA) {
            response[0] = 1;
            response[1] = 0x20;
            response[2] = 16;
            memcpy(&response[3], card.idm, 8);
            memcpy(&response[11], card.pmm, 8);
            send_response(request, 0, response, sizeof(response));
        } else {
            send_response(request, 0, response, 1);
        }
        return;
    }

    case SG_NFC_MIFARE_SELECT_TAG:
        if (payload_size != 4) {
            err = ESP_ERR_INVALID_SIZE;
        } else {
            err = reader_mifare_select(payload);
        }
        break;

    case SG_NFC_MIFARE_SET_KEY_AIME:
    case SG_NFC_MIFARE_SET_KEY_BANA:
        if (payload_size != 6) {
            err = ESP_ERR_INVALID_SIZE;
        } else {
            const uint8_t type = request[3] == SG_NFC_MIFARE_SET_KEY_AIME
                    ? READER_KEY_AIME
                    : READER_KEY_BANA;
            err = reader_mifare_set_key(type, payload);
        }
        break;

    case SG_NFC_MIFARE_AUTHENTICATE_AIME:
    case SG_NFC_MIFARE_AUTHENTICATE_BANA: {
        const uint8_t type = request[3] == SG_NFC_MIFARE_AUTHENTICATE_AIME
                ? READER_KEY_AIME
                : READER_KEY_BANA;
        err = reader_mifare_authenticate(type, payload, payload_size);
        break;
    }

    case SG_NFC_MIFARE_READ_BLOCK: {
        if (payload_size != 5) {
            err = ESP_ERR_INVALID_SIZE;
            break;
        }
        uint8_t block[16];
        err = reader_mifare_read_block(payload, payload[4], block);
        if (err == ESP_OK) {
            send_response(request, 0, block, sizeof(block));
            return;
        }
        break;
    }

    case SG_NFC_FELICA_ENCAP: {
        if (payload_size < 9 || payload_size != 8 + payload[8]) {
            err = ESP_ERR_INVALID_SIZE;
            break;
        }
        uint8_t response[250];
        size_t response_size = 0;
        err = reader_felica_transact(
                &payload[8],
                payload[8],
                response,
                sizeof(response),
                &response_size);
        if (err == ESP_OK) {
            send_response(request, 0, response, response_size);
            return;
        }
        break;
    }

    case SG_NFC_TO_UPDATE_MODE:
        break;

    case SG_NFC_SEND_HEX_DATA:
        send_response(request, payload_size == 0x2B ? 0x20 : 0, NULL, 0);
        return;

    default:
        send_response(request, 1, NULL, 0);
        return;
    }

    send_response(request, err == ESP_OK ? 0 : 1, NULL, 0);
}

static void dispatch(const uint8_t *request, size_t size)
{
    if (size < 5 || request[0] != size || request[4] != size - 5) {
        return;
    }

    const uint8_t *payload = &request[5];
    const size_t payload_size = request[4];
    if (request[1] == SG_ADDRESS_NFC) {
        dispatch_nfc(request, payload, payload_size);
    } else if (request[1] == SG_ADDRESS_LED) {
        dispatch_led(request, payload, payload_size);
    }
}

static void reset_parser(void)
{
    parser.synchronized = false;
    parser.escaped = false;
    parser.size = 0;
}

void sega_init(void)
{
    reset_parser();
}

void sega_receive(const uint8_t *data, size_t size)
{
    for (size_t i = 0; i < size; ++i) {
        uint8_t byte = data[i];
        if (!parser.synchronized) {
            if (byte == SG_SYNC) {
                parser.synchronized = true;
                parser.size = 0;
            }
            continue;
        }

        if (!parser.escaped && byte == SG_SYNC) {
            parser.size = 0;
            continue;
        }
        if (!parser.escaped && byte == SG_ESCAPE) {
            parser.escaped = true;
            continue;
        }
        if (parser.escaped) {
            byte++;
            parser.escaped = false;
        }

        if (parser.size >= sizeof(parser.bytes)) {
            reset_parser();
            continue;
        }
        parser.bytes[parser.size++] = byte;

        if (parser.size > 0) {
            const size_t expected = (size_t) parser.bytes[0] + 1;
            if (parser.size == expected) {
                uint8_t checksum = 0;
                for (size_t j = 0; j + 1 < parser.size; ++j) {
                    checksum += parser.bytes[j];
                }
                if (checksum == parser.bytes[parser.size - 1]) {
                    dispatch(parser.bytes, parser.size - 1);
                }
                reset_parser();
            } else if (parser.size > expected) {
                reset_parser();
            }
        }
    }
}
