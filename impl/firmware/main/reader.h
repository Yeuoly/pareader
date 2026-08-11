#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

typedef enum {
    READER_CARD_NONE = 0,
    READER_CARD_MIFARE = 1,
    READER_CARD_FELICA = 2,
} reader_card_type_t;

typedef struct {
    reader_card_type_t type;
    uint8_t mifare_uid[4];
    uint8_t luid[10];
    uint8_t blocks[32];
    uint8_t idm[8];
    uint8_t pmm[8];
    bool has_blocks;
} reader_card_t;

enum {
    READER_KEY_AIME = 0,
    READER_KEY_BANA = 1,
};

esp_err_t reader_init(void);
esp_err_t reader_read_card(reader_card_t *card);
esp_err_t reader_mifare_select(const uint8_t uid[4]);
esp_err_t reader_mifare_set_key(uint8_t key_type, const uint8_t key[6]);
esp_err_t reader_mifare_authenticate(
        uint8_t key_type,
        const uint8_t *payload,
        size_t payload_size);
esp_err_t reader_mifare_read_block(
        const uint8_t uid[4],
        uint8_t block_number,
        uint8_t block[16]);
esp_err_t reader_felica_transact(
        const uint8_t *request,
        size_t request_size,
        uint8_t *response,
        size_t response_capacity,
        size_t *response_size);
esp_err_t reader_radio_set(bool enabled);
void reader_set_led(uint8_t red, uint8_t green, uint8_t blue);
