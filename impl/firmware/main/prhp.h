#pragma once

#include <stddef.h>
#include <stdint.h>

#include "reader.h"

#define PRHP_REPORT_SIZE 64

void prhp_dispatch(const uint8_t request[PRHP_REPORT_SIZE]);
void prhp_publish_card_state(const reader_card_t *card);
