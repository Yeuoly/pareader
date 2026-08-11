#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

typedef struct {
    uint8_t target;
    uint8_t uid[10];
    size_t uid_size;
    uint8_t sak;
    uint16_t atqa;
} pn532_type_a_target_t;

typedef struct {
    uint8_t target;
    uint8_t idm[8];
    uint8_t pmm[8];
} pn532_felica_target_t;

esp_err_t pn532_init(void);
esp_err_t pn532_set_radio(bool enabled);
esp_err_t pn532_poll_type_a(pn532_type_a_target_t *target, bool *found);
esp_err_t pn532_poll_felica(pn532_felica_target_t *target, bool *found);
esp_err_t pn532_mifare_authenticate(
        uint8_t target,
        uint8_t command,
        uint8_t block,
        const uint8_t key[6],
        const uint8_t uid[4]);
esp_err_t pn532_mifare_read_block(uint8_t target, uint8_t block, uint8_t data[16]);
esp_err_t pn532_felica_transact(
        uint8_t target,
        const uint8_t *request,
        size_t request_size,
        uint8_t *response,
        size_t response_capacity,
        size_t *response_size);
