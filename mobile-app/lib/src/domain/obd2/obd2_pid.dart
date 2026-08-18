import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// PIDs OBD-II do Serviço 0x01 (dados em tempo real) que o painel lê.
///
/// Cada valor carrega sua identidade no protocolo (`mode`/`pid`), o rótulo e a
/// unidade para exibição, e a **regra de negócio pura** de conversão dos bytes
/// crus em grandeza física ([decode]) — no mesmo espírito de
/// `BleSignalLevel.fromRssi`: a fórmula vive no domínio, isolada e testável, e
/// as camadas acima só a aplicam (repository) ou exibem (UI). A montagem
/// byte-a-byte do fio fica no datasource; aqui só entram bytes já extraídos.
///
/// Curadoria: só entram PIDs que decodificam para **um único valor físico
/// contínuo** (o contrato de [decode] é `List<int> -&gt; double`). PIDs de
/// status/bitmask (ex.: monitor status, fuel system status), categóricos
/// (ex.: tipo de combustível, padrão OBD), códigos/tabelas compostas (ex.:
/// freeze frame DTC, curva de torque) ou tetos de calibração (máximos de
/// fundo de escala) ficam de fora — não cabem nesse contrato.
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

  /// Ajuste de combustível de curto prazo, banco 1 (%). Oscila em torno de 0;
  /// valores distantes de 0 indicam mistura desregulada. Fórmula:
  /// 100·A/128 − 100.
  shortFuelTrim1(
    pid: 0x06,
    label: 'Ajuste de combustível curto prazo (Banco 1)',
    unit: '%',
    defaultMin: -25,
    defaultMax: 25,
    defaultLowMax: 10,
    defaultHighMin: 15,
  ),

  /// Ajuste de combustível de longo prazo, banco 1 (%). Fórmula:
  /// 100·A/128 − 100.
  longFuelTrim1(
    pid: 0x07,
    label: 'Ajuste de combustível longo prazo (Banco 1)',
    unit: '%',
    defaultMin: -25,
    defaultMax: 25,
    defaultLowMax: 10,
    defaultHighMin: 15,
  ),

  /// Pressão da linha de combustível (kPa, gauge). Fórmula: 3·A.
  fuelPressureGauge(
    pid: 0x0A,
    label: 'Pressão de combustível',
    unit: 'kPa',
    defaultMin: 0,
    defaultMax: 500,
    defaultLowMax: 350,
    defaultHighMin: 450,
  ),

  /// Pressão absoluta do coletor de admissão (kPa). Fórmula: A.
  intakeManifoldPressure(
    pid: 0x0B,
    label: 'Pressão absoluta do coletor de admissão',
    unit: 'kPa',
    defaultMin: 0,
    defaultMax: 200,
    defaultLowMax: 100,
    defaultHighMin: 150,
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

  /// Temperatura do ar de admissão (°C). Fórmula: A − 40.
  intakeAirTemp(
    pid: 0x0F,
    label: 'Temp. do ar de admissão',
    unit: '°C',
    defaultMin: -20,
    defaultMax: 80,
    defaultLowMax: 45,
    defaultHighMin: 60,
  ),

  /// Fluxo de massa de ar — MAF (g/s). Fórmula: (256·A + B) / 100.
  maf(
    pid: 0x10,
    label: 'Fluxo de massa de ar (MAF)',
    unit: 'g/s',
    defaultMin: 0,
    defaultMax: 60,
    defaultLowMax: 35,
    defaultHighMin: 50,
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
  ),

  /// Sonda lambda 1, banco 1 (tensão, V). O PID também carrega o ajuste de
  /// curto prazo associado num segundo byte, descartado aqui (já coberto por
  /// [shortFuelTrim1]). Fórmula: A / 200.
  o2Sensor1Voltage(
    pid: 0x14,
    label: 'Sonda lambda 1 (tensão)',
    unit: 'V',
    defaultMin: 0,
    defaultMax: 1,
    defaultLowMax: 0.6,
    defaultHighMin: 0.8,
  ),

  /// Sonda lambda 2, banco 1, pós-catalisador (tensão, V). Fórmula: A / 200.
  o2Sensor2Voltage(
    pid: 0x15,
    label: 'Sonda lambda 2 pós-catalisador (tensão)',
    unit: 'V',
    defaultMin: 0,
    defaultMax: 1,
    defaultLowMax: 0.6,
    defaultHighMin: 0.8,
  ),

  /// Tempo de funcionamento do motor desde a partida (s). Fórmula:
  /// 256·A + B.
  engineRunTime(
    pid: 0x1F,
    label: 'Tempo de funcionamento do motor',
    unit: 's',
    defaultMin: 0,
    defaultMax: 3600,
    defaultLowMax: 1800,
    defaultHighMin: 3000,
  ),

  /// Distância percorrida com a luz de falha (MIL) acesa (km). Fórmula:
  /// 256·A + B.
  distanceWithMil(
    pid: 0x21,
    label: 'Distância percorrida com MIL acesa',
    unit: 'km',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 20,
    defaultHighMin: 50,
  ),

  /// EGR comandada (%). Fórmula: 100·A/255.
  commandedEgr(
    pid: 0x2C,
    label: 'EGR comandada',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 40,
    defaultHighMin: 70,
  ),

  /// Erro de EGR (%) — oscila em torno de 0. Fórmula: 100·A/128 − 100.
  egrError(
    pid: 0x2D,
    label: 'Erro de EGR',
    unit: '%',
    defaultMin: -25,
    defaultMax: 25,
    defaultLowMax: 10,
    defaultHighMin: 15,
  ),

  /// Purga evaporativa comandada (%). Fórmula: 100·A/255.
  commandedEvapPurge(
    pid: 0x2E,
    label: 'Purga evaporativa comandada',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 50,
    defaultHighMin: 80,
  ),

  /// Nível do tanque de combustível (%). Fórmula: 100·A/255.
  fuelTankLevel(
    pid: 0x2F,
    label: 'Nível do tanque de combustível',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 20,
    defaultHighMin: 40,
  ),

  /// Ciclos de aquecimento (warm-ups) desde o reset dos códigos — contagem
  /// crua, sem unidade. Fórmula: A.
  warmupsSinceClear(
    pid: 0x30,
    label: 'Ciclos de aquecimento desde reset',
    unit: '',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 20,
    defaultHighMin: 50,
  ),

  /// Distância percorrida desde o reset dos códigos (km). Fórmula:
  /// 256·A + B.
  distanceSinceClear(
    pid: 0x31,
    label: 'Distância desde reset de códigos',
    unit: 'km',
    defaultMin: 0,
    defaultMax: 2000,
    defaultLowMax: 500,
    defaultHighMin: 1000,
  ),

  /// Pressão de vapor do sistema evaporativo (Pa). Fórmula: (256·A + B) / 4.
  evapVaporPressure(
    pid: 0x32,
    label: 'Pressão de vapor evaporativo',
    unit: 'Pa',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 40,
    defaultHighMin: 70,
  ),

  /// Pressão barométrica absoluta (kPa). Fórmula: A.
  baroPressure(
    pid: 0x33,
    label: 'Pressão barométrica absoluta',
    unit: 'kPa',
    defaultMin: 80,
    defaultMax: 110,
    defaultLowMax: 95,
    defaultHighMin: 105,
  ),

  /// Temperatura do catalisador, banco 1 sensor 1 (°C). Fórmula:
  /// (256·A + B)/10 − 40.
  catalystTemp1(
    pid: 0x3C,
    label: 'Temp. do catalisador (banco 1, sensor 1)',
    unit: '°C',
    defaultMin: 0,
    defaultMax: 900,
    defaultLowMax: 500,
    defaultHighMin: 700,
  ),

  /// Tensão do módulo de controle (bateria/alternador, V). Fórmula:
  /// (256·A + B)/1000.
  controlModuleVoltage(
    pid: 0x42,
    label: 'Tensão do módulo de controle',
    unit: 'V',
    defaultMin: 8,
    defaultMax: 16,
    defaultLowMax: 12.5,
    defaultHighMin: 14.5,
  ),

  /// Carga absoluta do motor (%) — pode passar de 100% sob boost. Fórmula:
  /// 100·(256·A + B)/255.
  absoluteLoad(
    pid: 0x43,
    label: 'Carga absoluta do motor',
    unit: '%',
    defaultMin: 0,
    defaultMax: 150,
    defaultLowMax: 70,
    defaultHighMin: 110,
  ),

  /// Razão ar-combustível equivalente comandada (λ) — 1.0 = estequiométrica.
  /// Fórmula: 2·(256·A + B)/65536.
  commandedEquivRatio(
    pid: 0x44,
    label: 'Razão ar-combustível comandada (λ)',
    unit: 'λ',
    defaultMin: 0,
    defaultMax: 2,
    defaultLowMax: 0.9,
    defaultHighMin: 1.1,
  ),

  /// Posição relativa do acelerador (%). Fórmula: 100·A/255.
  relativeThrottle(
    pid: 0x45,
    label: 'Posição relativa do acelerador',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 40,
    defaultHighMin: 75,
  ),

  /// Temperatura do ar ambiente (°C). Fórmula: A − 40.
  ambientAirTemp(
    pid: 0x46,
    label: 'Temp. do ar ambiente',
    unit: '°C',
    defaultMin: -20,
    defaultMax: 50,
    defaultLowMax: 35,
    defaultHighMin: 42,
  ),

  /// Posição da borboleta B (%). Fórmula: 100·A/255.
  throttlePositionB(
    pid: 0x47,
    label: 'Posição da borboleta B',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 40,
    defaultHighMin: 75,
  ),

  /// Posição do pedal do acelerador, sensor D (%). Fórmula: 100·A/255.
  acceleratorPedalD(
    pid: 0x49,
    label: 'Posição do pedal do acelerador D',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 40,
    defaultHighMin: 75,
  ),

  /// Atuador de borboleta comandado (%). Fórmula: 100·A/255.
  commandedThrottleActuator(
    pid: 0x4C,
    label: 'Atuador de borboleta comandado',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 40,
    defaultHighMin: 75,
  ),

  /// Tempo total com a luz de falha (MIL) acesa (min). Fórmula: 256·A + B.
  timeMilOn(
    pid: 0x4D,
    label: 'Tempo com MIL acesa',
    unit: 'min',
    defaultMin: 0,
    defaultMax: 600,
    defaultLowMax: 100,
    defaultHighMin: 300,
  ),

  /// Tempo desde o reset dos códigos de falha (min). Fórmula: 256·A + B.
  timeSinceClear(
    pid: 0x4E,
    label: 'Tempo desde reset de códigos',
    unit: 'min',
    defaultMin: 0,
    defaultMax: 2000,
    defaultLowMax: 500,
    defaultHighMin: 1000,
  ),

  /// Percentual de etanol no combustível (%). Fórmula: 100·A/255.
  ethanolPercent(
    pid: 0x52,
    label: 'Percentual de etanol no combustível',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 30,
    defaultHighMin: 75,
  ),

  /// Posição relativa do pedal do acelerador (%). Fórmula: 100·A/255.
  relativeAcceleratorPedal(
    pid: 0x5A,
    label: 'Posição relativa do pedal do acelerador',
    unit: '%',
    defaultMin: 0,
    defaultMax: 100,
    defaultLowMax: 40,
    defaultHighMin: 75,
  ),

  /// Temperatura do óleo do motor (°C). Fórmula: A − 40.
  engineOilTemp(
    pid: 0x5C,
    label: 'Temp. do óleo do motor',
    unit: '°C',
    defaultMin: -20,
    defaultMax: 150,
    defaultLowMax: 110,
    defaultHighMin: 130,
  ),

  /// Temporização de injeção de combustível (°) — oscila em torno de 0.
  /// Fórmula: (256·A + B)/128 − 210.
  fuelInjectionTiming(
    pid: 0x5D,
    label: 'Temporização de injeção de combustível',
    unit: '°',
    defaultMin: -50,
    defaultMax: 50,
    defaultLowMax: 20,
    defaultHighMin: 35,
  ),

  /// Taxa de consumo de combustível (L/h). Fórmula: (256·A + B)/20.
  engineFuelRate(
    pid: 0x5E,
    label: 'Taxa de consumo de combustível',
    unit: 'L/h',
    defaultMin: 0,
    defaultMax: 40,
    defaultLowMax: 20,
    defaultHighMin: 30,
  ),

  /// Torque demandado pelo motorista, % do torque máximo do motor. Fórmula:
  /// A − 125.
  driverDemandTorque(
    pid: 0x61,
    label: 'Torque demandado pelo motorista',
    unit: '%',
    defaultMin: -25,
    defaultMax: 100,
    defaultLowMax: 50,
    defaultHighMin: 80,
  ),

  /// Torque real do motor, % do torque máximo. Fórmula: A − 125.
  actualEngineTorque(
    pid: 0x62,
    label: 'Torque real do motor',
    unit: '%',
    defaultMin: -25,
    defaultMax: 100,
    defaultLowMax: 50,
    defaultHighMin: 80,
  ),

  /// Torque de referência do motor (N·m). Fórmula: 256·A + B.
  engineReferenceTorque(
    pid: 0x63,
    label: 'Torque de referência do motor',
    unit: 'N·m',
    defaultMin: 0,
    defaultMax: 600,
    defaultLowMax: 400,
    defaultHighMin: 500,
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
      case Obd2Pid.shortFuelTrim1:
        return 100 * _a(data) / 128 - 100;
      case Obd2Pid.longFuelTrim1:
        return 100 * _a(data) / 128 - 100;
      case Obd2Pid.fuelPressureGauge:
        return (3 * _a(data)).toDouble();
      case Obd2Pid.intakeManifoldPressure:
        return _a(data).toDouble();
      case Obd2Pid.rpm:
        return ((_a(data) * 256) + _b(data)) / 4;
      case Obd2Pid.speed:
        return _a(data).toDouble();
      case Obd2Pid.timingAdvance:
        return _a(data) / 2 - 64;
      case Obd2Pid.intakeAirTemp:
        return _a(data) - 40;
      case Obd2Pid.maf:
        return ((_a(data) * 256) + _b(data)) / 100;
      case Obd2Pid.throttle:
        return _a(data) / 2.55;
      case Obd2Pid.o2Sensor1Voltage:
        return _a(data) / 200;
      case Obd2Pid.o2Sensor2Voltage:
        return _a(data) / 200;
      case Obd2Pid.engineRunTime:
        return ((_a(data) * 256) + _b(data)).toDouble();
      case Obd2Pid.distanceWithMil:
        return ((_a(data) * 256) + _b(data)).toDouble();
      case Obd2Pid.commandedEgr:
        return 100 * _a(data) / 255;
      case Obd2Pid.egrError:
        return 100 * _a(data) / 128 - 100;
      case Obd2Pid.commandedEvapPurge:
        return 100 * _a(data) / 255;
      case Obd2Pid.fuelTankLevel:
        return 100 * _a(data) / 255;
      case Obd2Pid.warmupsSinceClear:
        return _a(data).toDouble();
      case Obd2Pid.distanceSinceClear:
        return ((_a(data) * 256) + _b(data)).toDouble();
      case Obd2Pid.evapVaporPressure:
        return ((_a(data) * 256) + _b(data)) / 4;
      case Obd2Pid.baroPressure:
        return _a(data).toDouble();
      case Obd2Pid.catalystTemp1:
        return ((_a(data) * 256) + _b(data)) / 10 - 40;
      case Obd2Pid.controlModuleVoltage:
        return ((_a(data) * 256) + _b(data)) / 1000;
      case Obd2Pid.absoluteLoad:
        return 100 * ((_a(data) * 256) + _b(data)) / 255;
      case Obd2Pid.commandedEquivRatio:
        return 2 * ((_a(data) * 256) + _b(data)) / 65536;
      case Obd2Pid.relativeThrottle:
        return 100 * _a(data) / 255;
      case Obd2Pid.ambientAirTemp:
        return _a(data) - 40;
      case Obd2Pid.throttlePositionB:
        return 100 * _a(data) / 255;
      case Obd2Pid.acceleratorPedalD:
        return 100 * _a(data) / 255;
      case Obd2Pid.commandedThrottleActuator:
        return 100 * _a(data) / 255;
      case Obd2Pid.timeMilOn:
        return ((_a(data) * 256) + _b(data)).toDouble();
      case Obd2Pid.timeSinceClear:
        return ((_a(data) * 256) + _b(data)).toDouble();
      case Obd2Pid.ethanolPercent:
        return 100 * _a(data) / 255;
      case Obd2Pid.relativeAcceleratorPedal:
        return 100 * _a(data) / 255;
      case Obd2Pid.engineOilTemp:
        return _a(data) - 40;
      case Obd2Pid.fuelInjectionTiming:
        return ((_a(data) * 256) + _b(data)) / 128 - 210;
      case Obd2Pid.engineFuelRate:
        return ((_a(data) * 256) + _b(data)) / 20;
      case Obd2Pid.driverDemandTorque:
        return (_a(data) - 125).toDouble();
      case Obd2Pid.actualEngineTorque:
        return (_a(data) - 125).toDouble();
      case Obd2Pid.engineReferenceTorque:
        return ((_a(data) * 256) + _b(data)).toDouble();
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
