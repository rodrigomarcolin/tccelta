#include "TwaiCan.h"
#include <cstring>

// TODO: Revisar. Este arquivo foi gerado com IA.
TwaiCan::TwaiCan(int txPin, int rxPin) : _txPin(txPin), _rxPin(rxPin) {}

bool TwaiCan::begin() {
    twai_general_config_t gConfig = TWAI_GENERAL_CONFIG_DEFAULT(
        (gpio_num_t)_txPin, (gpio_num_t)_rxPin, TWAI_MODE_NORMAL);
    twai_timing_config_t  tConfig = TWAI_TIMING_CONFIG_500KBITS();
    twai_filter_config_t  fConfig = TWAI_FILTER_CONFIG_ACCEPT_ALL();

    if (twai_driver_install(&gConfig, &tConfig, &fConfig) != ESP_OK) return false;
    return twai_start() == ESP_OK;
}

bool TwaiCan::send(const CanFrame& frame) {
    twai_message_t msg = {};
    msg.identifier       = frame.id;
    msg.data_length_code = frame.dlc;
    memcpy(msg.data, frame.data, frame.dlc);
    return twai_transmit(&msg, pdMS_TO_TICKS(10)) == ESP_OK;
}

bool TwaiCan::receive(CanFrame& frame) {
    twai_message_t msg;
    // Non-blocking: timeout = 0 ticks
    if (twai_receive(&msg, 0) != ESP_OK) return false;
    frame.id  = msg.identifier;
    frame.dlc = msg.data_length_code;
    memcpy(frame.data, msg.data, msg.data_length_code);
    return true;
}
