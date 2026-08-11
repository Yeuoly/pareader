#include "reader.h"

#include <string.h>

#include "esp_check.h"

#include "pn532.h"

static const uint8_t default_aime_key[6] = {0x57, 0x43, 0x43, 0x46, 0x76, 0x32};
static const uint8_t default_bana_key[6] = {0x60, 0x90, 0xD0, 0x06, 0x32, 0xF5};

enum {
    FELICA_SYSTEM_CODE_TRANSIT = 0x0003,
    FELICA_SYSTEM_CODE_WILDCARD = 0xFFFF,
};

static uint8_t mifare_keys[2][6];
static uint8_t active_key_type;
static pn532_type_a_target_t active_mifare;
static pn532_felica_target_t active_felica;
static uint16_t felica_system_code = FELICA_SYSTEM_CODE_TRANSIT;
static bool radio_enabled = true;

static bool valid_luid(const uint8_t luid[10])
{
    bool nonzero = false;
    for (size_t i = 0; i < 10; ++i) {
        if ((luid[i] >> 4) > 9 || (luid[i] & 0x0F) > 9) {
            return false;
        }
        nonzero |= luid[i] != 0;
    }
    return nonzero && (luid[0] >> 4) != 3;
}

static esp_err_t read_mifare_with_key(
        const pn532_type_a_target_t *target,
        uint8_t auth_command,
        uint8_t auth_block,
        const uint8_t key[6],
        reader_card_t *card)
{
    ESP_RETURN_ON_ERROR(
            pn532_mifare_authenticate(
                    target->target,
                    auth_command,
                    auth_block,
                    key,
                    target->uid),
            "reader",
            "MIFARE authentication failed");
    ESP_RETURN_ON_ERROR(
            pn532_mifare_read_block(target->target, 1, &card->blocks[0]),
            "reader",
            "MIFARE block 1 failed");
    ESP_RETURN_ON_ERROR(
            pn532_mifare_read_block(target->target, 2, &card->blocks[16]),
            "reader",
            "MIFARE block 2 failed");
    card->has_blocks = true;
    return ESP_OK;
}

esp_err_t reader_init(void)
{
    memcpy(mifare_keys[READER_KEY_AIME], default_aime_key, sizeof(default_aime_key));
    memcpy(mifare_keys[READER_KEY_BANA], default_bana_key, sizeof(default_bana_key));

    return pn532_init();
}

esp_err_t reader_read_card(reader_card_t *card)
{
    memset(card, 0, sizeof(*card));
    if (!radio_enabled) {
        return ESP_OK;
    }

    bool found = false;
    esp_err_t err = pn532_poll_felica(felica_system_code, &active_felica, &found);
    if (err == ESP_OK) {
        if (found) {
            card->type = READER_CARD_FELICA;
            memcpy(card->idm, active_felica.idm, sizeof(card->idm));
            memcpy(card->pmm, active_felica.pmm, sizeof(card->pmm));
            return ESP_OK;
        }
        felica_system_code = felica_system_code == FELICA_SYSTEM_CODE_TRANSIT
                ? FELICA_SYSTEM_CODE_WILDCARD
                : FELICA_SYSTEM_CODE_TRANSIT;
    }

    ESP_RETURN_ON_ERROR(pn532_poll_type_a(&active_mifare, &found), "reader", "Type A poll failed");
    if (!found || active_mifare.uid_size != 4) {
        return ESP_OK;
    }

    card->type = READER_CARD_MIFARE;
    memcpy(card->mifare_uid, active_mifare.uid, sizeof(card->mifare_uid));

    err = read_mifare_with_key(
            &active_mifare,
            0x61,
            2,
            mifare_keys[READER_KEY_AIME],
        card);
    if (err == ESP_OK) {
        memcpy(card->luid, &card->blocks[16 + 6], sizeof(card->luid));
        if (!valid_luid(card->luid)) {
            memset(card->luid, 0, sizeof(card->luid));
        }
        return ESP_OK;
    }

    memset(card->blocks, 0, sizeof(card->blocks));
    card->has_blocks = false;
    err = read_mifare_with_key(
            &active_mifare,
            0x60,
            1,
            mifare_keys[READER_KEY_BANA],
            card);
    // An unsupported Type A tag is a successful poll with no PRHP card data.
    // The UID remains available to the official SEGA CDC poll path.
    (void) err;
    return ESP_OK;
}

esp_err_t reader_mifare_select(const uint8_t uid[4])
{
    if (active_mifare.uid_size != 4 || memcmp(active_mifare.uid, uid, 4) != 0) {
        return ESP_ERR_NOT_FOUND;
    }
    return ESP_OK;
}

esp_err_t reader_mifare_set_key(uint8_t key_type, const uint8_t key[6])
{
    if (key_type > READER_KEY_BANA) {
        return ESP_ERR_INVALID_ARG;
    }
    memcpy(mifare_keys[key_type], key, 6);
    return ESP_OK;
}

esp_err_t reader_mifare_authenticate(
        uint8_t key_type,
        const uint8_t *payload,
        size_t payload_size)
{
    (void) payload;
    (void) payload_size;
    if (key_type > READER_KEY_BANA) {
        return ESP_ERR_INVALID_ARG;
    }
    active_key_type = key_type;
    return ESP_OK;
}

esp_err_t reader_mifare_read_block(
        const uint8_t uid[4],
        uint8_t block_number,
        uint8_t block[16])
{
    ESP_RETURN_ON_ERROR(reader_mifare_select(uid), "reader", "MIFARE UID mismatch");
    const uint8_t auth_command = active_key_type == READER_KEY_AIME ? 0x61 : 0x60;
    ESP_RETURN_ON_ERROR(
            pn532_mifare_authenticate(
                    active_mifare.target,
                    auth_command,
                    block_number,
                    mifare_keys[active_key_type],
                    active_mifare.uid),
            "reader",
            "MIFARE block authentication failed");
    return pn532_mifare_read_block(active_mifare.target, block_number, block);
}

esp_err_t reader_felica_transact(
        const uint8_t *request,
        size_t request_size,
        uint8_t *response,
        size_t response_capacity,
        size_t *response_size)
{
    return pn532_felica_transact(
            active_felica.target,
            request,
            request_size,
            response,
            response_capacity,
            response_size);
}

esp_err_t reader_radio_set(bool enabled)
{
    ESP_RETURN_ON_ERROR(pn532_set_radio(enabled), "reader", "radio switch failed");
    radio_enabled = enabled;
    return ESP_OK;
}

void reader_set_led(uint8_t red, uint8_t green, uint8_t blue)
{
    (void) red;
    (void) green;
    (void) blue;
}
