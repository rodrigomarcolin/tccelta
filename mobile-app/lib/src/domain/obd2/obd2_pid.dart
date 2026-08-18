import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// PIDs OBD-II do Serviço 0x01 (dados em tempo real) que o painel lê.
///
/// Cada valor carrega sua identidade no protocolo (`mode`/`pid`), o rótulo e a
/// unidade para exibição, e a **regra de negócio pura** de conversão dos bytes
/// crus em grandeza física ([decode]) — no mesmo espírito de
/// `BleSignalLevel.fromRssi`: a fórmula vive no domínio, isolada e testável, e
/// as camadas acima só a aplicam (repository) ou exibem (UI). A montagem
/// byte-a-byte do fio fica no datasource; aqui só entram bytes já extraídos.
enum Obd2Pid {
  /// Carga calculada do motor (%). Fórmula: A / 2.55.
  engineLoad(
    pid: 0x04,
    label: 'Carga do motor',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 60,
    defaultHighMin: 85,
  ),

  /// Temperatura do líquido de arrefecimento (°C). Fórmula: A − 40.
  coolantTemp(
    pid: 0x05,
    label: 'Temp. do líquido',
    unit: '°C',
    defaultMin: 0,
    defaultMax: 130,
    defaultLowMax: 95,
    defaultHighMin: 110,
  ),

  /// Rotação do motor (RPM). Fórmula: ((A×256) + B) / 4.
  rpm(
    pid: 0x0C,
    label: 'Rotação do motor',
    unit: 'RPM',
    defaultMin: 0,
    defaultMax: 8000,
    defaultLowMax: 5000,
    defaultHighMin: 6500,
  ),

  /// Velocidade do veículo (km/h). Fórmula: A.
  speed(
    pid: 0x0D,
    label: 'Velocidade',
    unit: 'km/h',
    defaultMin: 0,
    defaultMax: 220,
    defaultLowMax: 120,
    defaultHighMin: 180,
  ),

  /// Avanço de ignição (° antes do PMS). Fórmula: A/2 − 64.
  timingAdvance(
    pid: 0x0E,
    label: 'Avanço de ignição',
    unit: '°',
    defaultMin: -10,
    defaultMax: 60,
    defaultLowMax: 20,
    defaultHighMin: 40,
  ),

  /// Posição do acelerador (%). Fórmula: A / 2.55.
  throttle(
    pid: 0x11,
    label: 'Posição do acelerador',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 40,
    defaultHighMin: 75,
  );

  const Obd2Pid({
    required this.pid,
    required this.label,
    required this.unit,
    required this.defaultMin,
    required this.defaultMax,
    required this.defaultLowMax,
    required this.defaultHighMin,
  });

  /// Serviço/modo OBD-II. Todos os PIDs do painel são do Serviço 0x01
  /// (aquisição de dados em tempo real).
  static const int mode = 0x01;

  /// Byte do PID dentro do serviço (ex.: 0x0C = rotação do motor).
  final int pid;

  /// Rótulo legível (pt-BR) exibido no card.
  final String label;

  /// Unidade da grandeza (ex.: "RPM", "%", "°C").
  final String unit;

  /// Fundo de escala mínimo padrão para gauge/barra deste PID — ponto de
  /// partida razoável para a customização do usuário, não um limite físico
  /// rígido. @see [IndicatorDisplay.defaultFor]
  final double defaultMin;

  /// Fundo de escala máximo padrão para gauge/barra deste PID.
  final double defaultMax;

  /// Limite superior padrão da zona "baixo" (ciano) do gauge ponteiro deste
  /// PID, em valor absoluto (não fração).
  final double defaultLowMax;

  /// Limite inferior padrão da zona "alto" (vermelho) do gauge ponteiro deste
  /// PID — entre [defaultLowMax] e este valor é a zona "médio" (âmbar).
  final double defaultHighMin;

  /// Comando de texto ELM327 para requisitar este PID, ex.: `010C`.
  ///
  /// Sem terminador: o `Elm327Client` anexa o `\r`.
  String get command => '${_hex(mode)}${_hex(pid)}';

  /// Byte do modo na RESPOSTA OBD-II (serviço + 0x40), ex.: 0x41 para o 0x01.
  static const int responseMode = mode + 0x40;

  /// Mapeia um número de PID cru (ex.: 0x0C) no valor do enum, ou `null` se não
  /// for um PID que o painel sabe decodificar. Usado para filtrar a descoberta
  /// de capacidades ao subconjunto exibível.
  static Obd2Pid? fromByte(int pid) {
    for (final p in Obd2Pid.values) {
      if (p.pid == pid) return p;
    }
    return null;
  }

  /// Converte os [data] bytes crus (já extraídos da resposta) na grandeza
  /// física, aplicando a fórmula do PID.
  ///
  /// Lança [AppException] se faltarem bytes — é uma invariante quebrada
  /// (o datasource só deve entregar respostas com o cabeçalho validado).
  double decode(List<int> data) {
    switch (this) {
      case Obd2Pid.engineLoad:
        return _a(data) / 2.55;
      case Obd2Pid.coolantTemp:
        return _a(data) - 40;
      case Obd2Pid.rpm:
        return ((_a(data) * 256) + _b(data)) / 4;
      case Obd2Pid.speed:
        return _a(data).toDouble();
      case Obd2Pid.timingAdvance:
        return _a(data) / 2 - 64;
      case Obd2Pid.throttle:
        return _a(data) / 2.55;
    }
  }

  int _a(List<int> data) {
    if (data.isEmpty) {
      throw AppException('PID $command sem byte A na resposta');
    }
    return data[0];
  }

  int _b(List<int> data) {
    if (data.length < 2) {
      throw AppException('PID $command sem byte B na resposta');
    }
    return data[1];
  }

  static String _hex(int byte) =>
      byte.toRadixString(16).toUpperCase().padLeft(2, '0');
}
