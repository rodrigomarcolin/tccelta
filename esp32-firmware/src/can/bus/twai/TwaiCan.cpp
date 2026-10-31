#include "TwaiCan.h"
#include <cstring>

// TODO: Revisar. Este arquivo foi gerado com IA.
TwaiCan::TwaiCan(CanBitrate bitrate, int txPin, int rxPin)
    : _bitrate(bitrate), _txPin(txPin), _rxPin(rxPin) {}

bool TwaiCan::begin() {
    twai_general_config_t gConfig = TWAI_GENERAL_CONFIG_DEFAULT(
        (gpio_num_t)_txPin, (gpio_num_t)_rxPin, TWAI_MODE_NORMAL);
    twai_timing_config_t  tConfig = _bitrate == CanBitrate::Kbps500
                                        ? (twai_timing_config_t)TWAI_TIMING_CONFIG_500KBITS()
                                        : (twai_timing_config_t)TWAI_TIMING_CONFIG_250KBITS();
    twai_filter_config_t  fConfig = TWAI_FILTER_CONFIG_ACCEPT_ALL();

    if (twai_driver_install(&gConfig, &tConfig, &fConfig) != ESP_OK) return false;
    return twai_start() == ESP_OK;
}

bool TwaiCan::send(const CanFrame& frame) {
    twai_message_t msg = {};
    msg.identifier       = frame.id;
    msg.extd             = frame.extended ? 1 : 0;
    msg.data_length_code = frame.dlc;
    memcpy(msg.data, frame.data, frame.dlc);
    if (twai_transmit(&msg, pdMS_TO_TICKS(10)) == ESP_OK) return true;
    ++_txDropped;
    return false;
}

bool TwaiCan::status(CanStatus& out) const {
    twai_status_info_t info;
    if (twai_get_status_info(&info) != ESP_OK) return false;
    switch (info.state) {
        case TWAI_STATE_RUNNING:    out.state = CanStatus::State::Running;    break;
        case TWAI_STATE_BUS_OFF:    out.state = CanStatus::State::BusOff;     break;
        case TWAI_STATE_RECOVERING: out.state = CanStatus::State::Recovering; break;
        default:                    out.state = CanStatus::State::Stopped;    break;
    }
    out.txErrors  = info.tx_error_counter;
    out.rxErrors  = info.rx_error_counter;
    out.txFailed  = info.tx_failed_count;
    out.rxMissed  = info.rx_missed_count;
    out.rxOverrun = info.rx_overrun_count;
    out.arbLost   = info.arb_lost_count;
    out.busErrors = info.bus_error_count;
    out.txQueued  = info.msgs_to_tx;
    out.rxQueued  = info.msgs_to_rx;
    out.txDropped = _txDropped;
    return true;
}

bool TwaiCan::receive(CanFrame& frame) {
    twai_message_t msg;
    // Non-blocking: timeout = 0 ticks
    if (twai_receive(&msg, 0) != ESP_OK) return false;
    frame.id  = msg.identifier;
    frame.extended = msg.extd != 0;
    frame.dlc = msg.data_length_code;
    memcpy(frame.data, msg.data, msg.data_length_code);
    return true;
}
