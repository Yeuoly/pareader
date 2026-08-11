#pragma once

#include <stddef.h>
#include <stdint.h>

void sega_init(void);
void sega_receive(const uint8_t *data, size_t size);
