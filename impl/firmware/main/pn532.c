#include "pn532.h"

#include <string.h>

#include "driver/i2c_master.h"
#include "esp_check.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#define PN532_I2C_ADDRESS 0x24
#define PN532_I2C_FREQUENCY_HZ 100000
#define PN532_I2C_TRANSFER_TIMEOUT_MS 100
#define PN532_I2C_READY 0x01

#define PN532_HOST_TO_PN532 0xD4
#define PN532_PN532_TO_HOST 0xD5
#define PN532_COMMAND_SAM_CONFIGURATION 0x14
#define PN532_COMMAND_RF_CONFIGURATION 0x32
#define PN532_COMMAND_IN_DATA_EXCHANGE 0x40
#define PN532_COMMAND_IN_LIST_PASSIVE_TARGET 0x4A

#define PN532_FRAME_CAPACITY 280
#define PN532_COMMAND_TIMEOUT_MS 600

static i2c_master_bus_handle_t bus;
static i2c_master_dev_handle_t device;

static esp_err_t wait_ready(uint32_t timeout_ms)
{
    const int64_t deadline = esp_timer_get_time() + (int64_t) timeout_ms * 1000;
    while (esp_timer_get_time() < deadline) {
        uint8_t status = 0;
        const esp_err_t result = i2c_master_receive(
                device,
                &status,
                sizeof(status),
                PN532_I2C_TRANSFER_TIMEOUT_MS);
        // PN532 NACKs its I2C read address while a command is still running.
        // ESP-IDF reports that NACK as ESP_ERR_INVALID_STATE, so it means
        // "not ready yet" here rather than a broken bus.
        if (result == ESP_ERR_INVALID_STATE) {
            vTaskDelay(pdMS_TO_TICKS(2));
            continue;
        }
        ESP_RETURN_ON_ERROR(result, "pn532", "status read failed");
        if ((status & PN532_I2C_READY) != 0) {
            return ESP_OK;
        }
        vTaskDelay(pdMS_TO_TICKS(2));
    }
    return ESP_ERR_TIMEOUT;
}

static esp_err_t write_frame(uint8_t command, const uint8_t *payload, size_t payload_size)
{
    const size_t data_size = payload_size + 2;
    if (data_size > 254) {
        return ESP_ERR_INVALID_SIZE;
    }

    uint8_t frame[PN532_FRAME_CAPACITY] = {0};
    size_t position = 0;
    frame[position++] = 0x00;
    frame[position++] = 0x00;
    frame[position++] = 0xFF;
    frame[position++] = (uint8_t) data_size;
    frame[position++] = (uint8_t) (0U - data_size);
    frame[position++] = PN532_HOST_TO_PN532;
    frame[position++] = command;

    uint8_t checksum = PN532_HOST_TO_PN532 + command;
    if (payload_size > 0) {
        memcpy(&frame[position], payload, payload_size);
        for (size_t i = 0; i < payload_size; ++i) {
            checksum += payload[i];
        }
        position += payload_size;
    }
    frame[position++] = (uint8_t) (0U - checksum);
    frame[position++] = 0x00;

    return i2c_master_transmit(device, frame, position, PN532_I2C_TRANSFER_TIMEOUT_MS);
}

static esp_err_t read_data(uint8_t *data, size_t size)
{
    if (size > PN532_FRAME_CAPACITY) {
        return ESP_ERR_INVALID_SIZE;
    }

    uint8_t rx[PN532_FRAME_CAPACITY + 1] = {0};
    ESP_RETURN_ON_ERROR(
            i2c_master_receive(device, rx, size + 1, PN532_I2C_TRANSFER_TIMEOUT_MS),
            "pn532",
            "data read failed");
    if ((rx[0] & PN532_I2C_READY) == 0) {
        return ESP_ERR_INVALID_STATE;
    }
    memcpy(data, &rx[1], size);
    return ESP_OK;
}

static esp_err_t read_ack(void)
{
    static const uint8_t expected[] = {0x00, 0x00, 0xFF, 0x00, 0xFF, 0x00};
    uint8_t ack[sizeof(expected)] = {0};
    ESP_RETURN_ON_ERROR(read_data(ack, sizeof(ack)), "pn532", "ACK read failed");
    return memcmp(ack, expected, sizeof(expected)) == 0 ? ESP_OK : ESP_ERR_INVALID_RESPONSE;
}

static esp_err_t command(
        uint8_t command_code,
        const uint8_t *request,
        size_t request_size,
        uint8_t *response,
        size_t response_capacity,
        size_t *response_size)
{
    ESP_RETURN_ON_ERROR(write_frame(command_code, request, request_size), "pn532", "frame write failed");
    ESP_RETURN_ON_ERROR(wait_ready(PN532_COMMAND_TIMEOUT_MS), "pn532", "ACK timeout");
    ESP_RETURN_ON_ERROR(read_ack(), "pn532", "invalid ACK");
    ESP_RETURN_ON_ERROR(wait_ready(PN532_COMMAND_TIMEOUT_MS), "pn532", "response timeout");

    uint8_t frame[PN532_FRAME_CAPACITY] = {0};
    ESP_RETURN_ON_ERROR(read_data(frame, sizeof(frame)), "pn532", "response read failed");

    if (frame[0] != 0x00 || frame[1] != 0x00 || frame[2] != 0xFF) {
        return ESP_ERR_INVALID_RESPONSE;
    }
    const size_t data_size = frame[3];
    if (data_size < 2 || data_size == 0xFF || (uint8_t) (frame[3] + frame[4]) != 0) {
        return ESP_ERR_INVALID_RESPONSE;
    }
    if (5 + data_size + 2 > sizeof(frame)) {
        return ESP_ERR_INVALID_SIZE;
    }

    uint8_t checksum = 0;
    for (size_t i = 0; i < data_size; ++i) {
        checksum += frame[5 + i];
    }
    if ((uint8_t) (checksum + frame[5 + data_size]) != 0 || frame[5 + data_size + 1] != 0) {
        return ESP_ERR_INVALID_CRC;
    }
    if (frame[5] != PN532_PN532_TO_HOST || frame[6] != (uint8_t) (command_code + 1)) {
        return ESP_ERR_INVALID_RESPONSE;
    }

    const size_t payload_size = data_size - 2;
    if (payload_size > response_capacity) {
        return ESP_ERR_INVALID_SIZE;
    }
    if (payload_size > 0 && response != NULL) {
        memcpy(response, &frame[7], payload_size);
    }
    *response_size = payload_size;
    return ESP_OK;
}

esp_err_t pn532_init(void)
{
    const i2c_master_bus_config_t bus_config = {
        .i2c_port = -1,
        .sda_io_num = CONFIG_PAREADER_PN532_SDA_GPIO,
        .scl_io_num = CONFIG_PAREADER_PN532_SCL_GPIO,
        .clk_source = I2C_CLK_SRC_DEFAULT,
        .glitch_ignore_cnt = 7,
        .flags.enable_internal_pullup = true,
    };
    ESP_RETURN_ON_ERROR(
            i2c_new_master_bus(&bus_config, &bus),
            "pn532",
            "I2C bus initialization failed");

    const i2c_device_config_t device_config = {
        .dev_addr_length = I2C_ADDR_BIT_LEN_7,
        .device_address = PN532_I2C_ADDRESS,
        .scl_speed_hz = PN532_I2C_FREQUENCY_HZ,
        .scl_wait_us = 20000,
    };
    ESP_RETURN_ON_ERROR(
            i2c_master_bus_add_device(bus, &device_config, &device),
            "pn532",
            "I2C device initialization failed");

    ESP_RETURN_ON_ERROR(
            i2c_master_probe(bus, PN532_I2C_ADDRESS, PN532_I2C_TRANSFER_TIMEOUT_MS),
            "pn532",
            "PN532 not found at I2C address 0x24");

    vTaskDelay(pdMS_TO_TICKS(100));

    const uint8_t sam[] = {0x01, 0x14, 0x01};
    uint8_t response[8];
    size_t response_size = 0;
    ESP_RETURN_ON_ERROR(
            command(PN532_COMMAND_SAM_CONFIGURATION, sam, sizeof(sam), response, sizeof(response), &response_size),
            "pn532",
            "SAM configuration failed");

    const uint8_t retries[] = {0x05, 0xFF, 0x01, 0x00};
    return command(
            PN532_COMMAND_RF_CONFIGURATION,
            retries,
            sizeof(retries),
            response,
            sizeof(response),
            &response_size);
}

esp_err_t pn532_set_radio(bool enabled)
{
    const uint8_t request[] = {0x01, enabled ? 0x01 : 0x00};
    uint8_t response[8];
    size_t response_size = 0;
    return command(
            PN532_COMMAND_RF_CONFIGURATION,
            request,
            sizeof(request),
            response,
            sizeof(response),
            &response_size);
}

esp_err_t pn532_poll_type_a(pn532_type_a_target_t *target, bool *found)
{
    const uint8_t request[] = {0x01, 0x00};
    uint8_t response[64];
    size_t size = 0;
    ESP_RETURN_ON_ERROR(
            command(PN532_COMMAND_IN_LIST_PASSIVE_TARGET, request, sizeof(request), response, sizeof(response), &size),
            "pn532",
            "Type A poll failed");

    *found = size > 0 && response[0] > 0;
    if (!*found) {
        return ESP_OK;
    }
    if (size < 6) {
        return ESP_ERR_INVALID_RESPONSE;
    }

    const size_t uid_size = response[5];
    if (uid_size > sizeof(target->uid) || size < 6 + uid_size) {
        return ESP_ERR_INVALID_RESPONSE;
    }
    target->target = response[1];
    target->atqa = ((uint16_t) response[2] << 8) | response[3];
    target->sak = response[4];
    target->uid_size = uid_size;
    memcpy(target->uid, &response[6], uid_size);
    return ESP_OK;
}

esp_err_t pn532_poll_felica(pn532_felica_target_t *target, bool *found)
{
    const uint8_t request[] = {0x01, 0x01, 0x00, 0xFF, 0xFF, 0x01, 0x00};
    uint8_t response[64];
    size_t size = 0;
    ESP_RETURN_ON_ERROR(
            command(PN532_COMMAND_IN_LIST_PASSIVE_TARGET, request, sizeof(request), response, sizeof(response), &size),
            "pn532",
            "FeliCa poll failed");

    *found = size > 0 && response[0] > 0;
    if (!*found) {
        return ESP_OK;
    }
    if (size < 21 || response[2] < 18) {
        return ESP_ERR_INVALID_RESPONSE;
    }

    target->target = response[1];
    memcpy(target->idm, &response[4], sizeof(target->idm));
    memcpy(target->pmm, &response[12], sizeof(target->pmm));
    return ESP_OK;
}

esp_err_t pn532_mifare_authenticate(
        uint8_t target,
        uint8_t mifare_command,
        uint8_t block,
        const uint8_t key[6],
        const uint8_t uid[4])
{
    uint8_t request[13] = {target, mifare_command, block};
    memcpy(&request[3], key, 6);
    memcpy(&request[9], uid, 4);

    uint8_t response[8];
    size_t size = 0;
    ESP_RETURN_ON_ERROR(
            command(PN532_COMMAND_IN_DATA_EXCHANGE, request, sizeof(request), response, sizeof(response), &size),
            "pn532",
            "MIFARE authentication failed");
    return size >= 1 && response[0] == 0 ? ESP_OK : ESP_ERR_INVALID_RESPONSE;
}

esp_err_t pn532_mifare_read_block(uint8_t target, uint8_t block, uint8_t data[16])
{
    const uint8_t request[] = {target, 0x30, block};
    uint8_t response[20];
    size_t size = 0;
    ESP_RETURN_ON_ERROR(
            command(PN532_COMMAND_IN_DATA_EXCHANGE, request, sizeof(request), response, sizeof(response), &size),
            "pn532",
            "MIFARE block read failed");
    if (size < 17 || response[0] != 0) {
        return ESP_ERR_INVALID_RESPONSE;
    }
    memcpy(data, &response[1], 16);
    return ESP_OK;
}

esp_err_t pn532_felica_transact(
        uint8_t target,
        const uint8_t *request,
        size_t request_size,
        uint8_t *response,
        size_t response_capacity,
        size_t *response_size)
{
    if (request_size + 1 > 254) {
        return ESP_ERR_INVALID_SIZE;
    }

    uint8_t payload[255];
    payload[0] = target;
    memcpy(&payload[1], request, request_size);

    uint8_t raw_response[255];
    size_t raw_size = 0;
    ESP_RETURN_ON_ERROR(
            command(
                    PN532_COMMAND_IN_DATA_EXCHANGE,
                    payload,
                    request_size + 1,
                    raw_response,
                    sizeof(raw_response),
                    &raw_size),
            "pn532",
            "FeliCa transaction failed");
    if (raw_size < 1 || raw_response[0] != 0 || raw_size - 1 > response_capacity) {
        return ESP_ERR_INVALID_RESPONSE;
    }

    *response_size = raw_size - 1;
    memcpy(response, &raw_response[1], *response_size);
    return ESP_OK;
}
