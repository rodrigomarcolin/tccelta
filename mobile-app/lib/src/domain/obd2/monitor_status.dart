/// Decodifica o byte A do PID 0x01 (Serviço 01, status de monitoramento):
/// bit 7 é o MIL, bits 0-6 são a contagem de DTCs confirmados. Fica fora do
/// enum `Obd2Pid` porque é um bitmask, não um valor físico contínuo (ver
/// nota em `obd2_pid.dart`).
///
/// Regra pura, testável, sem tocar em BLE — mesmo espírito de
/// `dtcCodeFromRaw`/`Obd2Pid.decode`.
bool monitorStatusMilOn(List<int> payload) =>
    payload.isNotEmpty && (payload.first & 0x80) != 0;
