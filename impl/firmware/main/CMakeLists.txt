idf_component_register(
    SRCS
        "app_main.c"
        "cardio.c"
        "pn532.c"
        "prhp.c"
        "reader.c"
        "sega.c"
        "usb_device.c"
    INCLUDE_DIRS "."
    REQUIRES esp_driver_gpio esp_driver_i2c esp_timer
)
