import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_freeze_frame_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/domain/repositories/dtc_repository.dart';

/// Implementação mockada de [DtcRepository]: devolve um catálogo estático de
/// códigos (curadoria pt-BR realista, alinhada ao design), sem tocar em BLE.
///
/// **Placeholder para a leitura real.** Quando o datasource ELM327 existir,
/// ele lerá os Modos 03 (confirmados)/07 (pendentes)/0A (permanentes) para os
/// códigos e o Modo 02 para o congelamento — a troca é só esta classe (e o
/// `dtcRepositoryProvider` em `diagnostics_providers.dart`), sem tocar em
/// domain/view_model/view.
class FakeDtcRepositoryImpl implements DtcRepository {
  /// Atraso simulado da leitura, para a UI exercitar o estado de carregamento
  /// (inicial e no "Reler") como uma leitura real faria.
  static const Duration _simulatedLatency = Duration(milliseconds: 600);

  @override
  Future<DtcSnapshot> read() async {
    await Future<void>.delayed(_simulatedLatency);
    const codes = _catalog;
    return DtcSnapshot(
      codes: codes,
      milOn: codes.any((c) => c.status == DtcStatus.confirmed),
    );
  }

  static const List<DtcCode> _catalog = [
    DtcCode(
      code: 'P0301',
      component: DtcComponent.engine,
      name: 'Falha de combustão — cilindro 1',
      severity: DtcSeverity.high,
      status: DtcStatus.confirmed,
      detectedLabel: 'há 2 dias · 3 ciclos',
      causes: [
        'Vela ou bobina do cilindro 1',
        'Bico injetor sujo ou travado',
        'Vazamento de admissão',
      ],
      freezeFrame: [
        DtcFreezeFrameEntry(label: 'Rotação', value: '2.480 rpm'),
        DtcFreezeFrameEntry(label: 'Velocidade', value: '58 km/h'),
        DtcFreezeFrameEntry(label: 'Temp. arrefec.', value: '92 °C'),
        DtcFreezeFrameEntry(label: 'Carga', value: '46 %'),
      ],
    ),
    DtcCode(
      code: 'P0171',
      component: DtcComponent.engine,
      name: 'Mistura pobre — banco 1',
      severity: DtcSeverity.medium,
      status: DtcStatus.pending,
      detectedLabel: 'neste ciclo de condução',
      causes: [
        'Entrada de ar falsa',
        'Sonda lambda fora de faixa',
        'Pressão de combustível baixa',
      ],
    ),
    DtcCode(
      code: 'P0420',
      component: DtcComponent.emissions,
      name: 'Eficiência do catalisador abaixo do limite',
      severity: DtcSeverity.medium,
      status: DtcStatus.confirmed,
      detectedLabel: 'há 11 dias',
      causes: [
        'Catalisador desgastado',
        'Sonda pós-catalisador',
        'Falha de combustão prolongada',
      ],
      freezeFrame: [
        DtcFreezeFrameEntry(label: 'Rotação', value: '1.960 rpm'),
        DtcFreezeFrameEntry(label: 'Velocidade', value: '74 km/h'),
        DtcFreezeFrameEntry(label: 'Temp. arrefec.', value: '95 °C'),
        DtcFreezeFrameEntry(label: 'Carga', value: '31 %'),
      ],
    ),
    DtcCode(
      code: 'U0121',
      component: DtcComponent.network,
      name: 'Perda de comunicação com o módulo do ABS',
      severity: DtcSeverity.high,
      status: DtcStatus.confirmed,
      detectedLabel: 'há 4 h',
      causes: [
        'Conector do módulo ABS',
        'Interrupção na rede CAN',
        'Alimentação do módulo',
      ],
      freezeFrame: [
        DtcFreezeFrameEntry(label: 'Tensão', value: '12,4 V'),
        DtcFreezeFrameEntry(label: 'Rotação', value: '820 rpm'),
        DtcFreezeFrameEntry(label: 'Velocidade', value: '0 km/h'),
        DtcFreezeFrameEntry(label: 'Temp. arrefec.', value: '88 °C'),
      ],
    ),
    DtcCode(
      code: 'B1200',
      component: DtcComponent.body,
      name: 'Motor do vidro elétrico diant. esq.',
      severity: DtcSeverity.low,
      status: DtcStatus.pending,
      detectedLabel: 'neste ciclo de condução',
      causes: ['Motor do vidro travado', 'Fiação da porta', 'Módulo da porta'],
    ),
    DtcCode(
      code: 'P0300',
      component: DtcComponent.engine,
      name: 'Falha de combustão aleatória',
      severity: DtcSeverity.high,
    ),
    DtcCode(
      code: 'P0302',
      component: DtcComponent.engine,
      name: 'Falha de combustão — cilindro 2',
      severity: DtcSeverity.high,
    ),
    DtcCode(
      code: 'P0102',
      component: DtcComponent.engine,
      name: 'Sinal baixo do sensor MAF',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'P0128',
      component: DtcComponent.engine,
      name: 'Termostato abaixo da temperatura',
      severity: DtcSeverity.low,
    ),
    DtcCode(
      code: 'P0335',
      component: DtcComponent.engine,
      name: 'Sensor de rotação do virabrequim',
      severity: DtcSeverity.high,
    ),
    DtcCode(
      code: 'P0230',
      component: DtcComponent.engine,
      name: 'Circuito da bomba de combustível',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'P0130',
      component: DtcComponent.emissions,
      name: 'Sonda lambda 1 — banco 1',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'P0442',
      component: DtcComponent.emissions,
      name: 'Vazamento pequeno no sistema EVAP',
      severity: DtcSeverity.low,
    ),
    DtcCode(
      code: 'P0455',
      component: DtcComponent.emissions,
      name: 'Vazamento grande no sistema EVAP',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'P0401',
      component: DtcComponent.emissions,
      name: 'Fluxo insuficiente de EGR',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'P0700',
      component: DtcComponent.transmission,
      name: 'Falha no módulo da transmissão',
      severity: DtcSeverity.high,
    ),
    DtcCode(
      code: 'P0715',
      component: DtcComponent.transmission,
      name: 'Sensor de rotação de entrada',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'P0740',
      component: DtcComponent.transmission,
      name: 'Conversor de torque — embreagem',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'C0035',
      component: DtcComponent.brakes,
      name: 'Sensor de roda diant. esquerda',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'C0040',
      component: DtcComponent.brakes,
      name: 'Sensor de roda diant. direita',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'C0561',
      component: DtcComponent.brakes,
      name: 'ABS desativado pelo sistema',
      severity: DtcSeverity.high,
    ),
    DtcCode(
      code: 'B1318',
      component: DtcComponent.body,
      name: 'Tensão de bateria baixa',
      severity: DtcSeverity.medium,
    ),
    DtcCode(
      code: 'B1250',
      component: DtcComponent.body,
      name: 'Trava elétrica — porta traseira dir.',
      severity: DtcSeverity.low,
    ),
    DtcCode(
      code: 'B2477',
      component: DtcComponent.body,
      name: 'Configuração do módulo de conforto',
      severity: DtcSeverity.low,
    ),
    DtcCode(
      code: 'U0100',
      component: DtcComponent.network,
      name: 'Perda de comunicação com a ECM',
      severity: DtcSeverity.high,
    ),
    DtcCode(
      code: 'U0155',
      component: DtcComponent.network,
      name: 'Perda de comunicação com o painel',
      severity: DtcSeverity.medium,
    ),
  ];
}
