import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_definition.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';

/// Catálogo de DTCs conhecidos pelo app — curadoria pt-BR realista, alinhada
/// ao design.
///
/// Dado de referência **permanente**: não vem de nenhuma leitura do veículo
/// (por isso não é buscado por nenhum repository/datasource), e sobrevive à
/// troca do repository mockado pelo real — é a partir daqui que a vista
/// "Todos por componente" é montada, decorada pelo resultado de
/// `Obd2Repository.readDtc()` (ver `DtcCode.fromDefinition`).
const List<DtcDefinition> dtcCatalog = [
  DtcDefinition(
    code: 'P0301',
    component: DtcComponent.engine,
    name: 'Falha de combustão — cilindro 1',
    severity: DtcSeverity.high,
    causes: [
      'Vela ou bobina do cilindro 1',
      'Bico injetor sujo ou travado',
      'Vazamento de admissão',
    ],
  ),
  DtcDefinition(
    code: 'P0171',
    component: DtcComponent.engine,
    name: 'Mistura pobre — banco 1',
    severity: DtcSeverity.medium,
    causes: [
      'Entrada de ar falsa',
      'Sonda lambda fora de faixa',
      'Pressão de combustível baixa',
    ],
  ),
  DtcDefinition(
    code: 'P0133',
    component: DtcComponent.emissions,
    name: 'Sonda lambda 1 — resposta lenta (banco 1)',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'P0420',
    component: DtcComponent.emissions,
    name: 'Eficiência do catalisador abaixo do limite',
    severity: DtcSeverity.medium,
    causes: [
      'Catalisador desgastado',
      'Sonda pós-catalisador',
      'Falha de combustão prolongada',
    ],
  ),
  DtcDefinition(
    code: 'U0121',
    component: DtcComponent.network,
    name: 'Perda de comunicação com o módulo do ABS',
    severity: DtcSeverity.high,
    causes: [
      'Conector do módulo ABS',
      'Interrupção na rede CAN',
      'Alimentação do módulo',
    ],
  ),
  DtcDefinition(
    code: 'B1200',
    component: DtcComponent.body,
    name: 'Motor do vidro elétrico diant. esq.',
    severity: DtcSeverity.low,
    causes: ['Motor do vidro travado', 'Fiação da porta', 'Módulo da porta'],
  ),
  DtcDefinition(
    code: 'P0300',
    component: DtcComponent.engine,
    name: 'Falha de combustão aleatória',
    severity: DtcSeverity.high,
  ),
  DtcDefinition(
    code: 'P0302',
    component: DtcComponent.engine,
    name: 'Falha de combustão — cilindro 2',
    severity: DtcSeverity.high,
  ),
  DtcDefinition(
    code: 'P0102',
    component: DtcComponent.engine,
    name: 'Sinal baixo do sensor MAF',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'P0128',
    component: DtcComponent.engine,
    name: 'Termostato abaixo da temperatura',
    severity: DtcSeverity.low,
  ),
  DtcDefinition(
    code: 'P0335',
    component: DtcComponent.engine,
    name: 'Sensor de rotação do virabrequim',
    severity: DtcSeverity.high,
  ),
  DtcDefinition(
    code: 'P0230',
    component: DtcComponent.engine,
    name: 'Circuito da bomba de combustível',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'P0130',
    component: DtcComponent.emissions,
    name: 'Sonda lambda 1 — banco 1',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'P0442',
    component: DtcComponent.emissions,
    name: 'Vazamento pequeno no sistema EVAP',
    severity: DtcSeverity.low,
  ),
  DtcDefinition(
    code: 'P0455',
    component: DtcComponent.emissions,
    name: 'Vazamento grande no sistema EVAP',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'P0401',
    component: DtcComponent.emissions,
    name: 'Fluxo insuficiente de EGR',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'P0700',
    component: DtcComponent.transmission,
    name: 'Falha no módulo da transmissão',
    severity: DtcSeverity.high,
  ),
  DtcDefinition(
    code: 'P0715',
    component: DtcComponent.transmission,
    name: 'Sensor de rotação de entrada',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'P0740',
    component: DtcComponent.transmission,
    name: 'Conversor de torque — embreagem',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'C0035',
    component: DtcComponent.brakes,
    name: 'Sensor de roda diant. esquerda',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'C0040',
    component: DtcComponent.brakes,
    name: 'Sensor de roda diant. direita',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'C0561',
    component: DtcComponent.brakes,
    name: 'ABS desativado pelo sistema',
    severity: DtcSeverity.high,
  ),
  DtcDefinition(
    code: 'B1318',
    component: DtcComponent.body,
    name: 'Tensão de bateria baixa',
    severity: DtcSeverity.medium,
  ),
  DtcDefinition(
    code: 'B1250',
    component: DtcComponent.body,
    name: 'Trava elétrica — porta traseira dir.',
    severity: DtcSeverity.low,
  ),
  DtcDefinition(
    code: 'B2477',
    component: DtcComponent.body,
    name: 'Configuração do módulo de conforto',
    severity: DtcSeverity.low,
  ),
  DtcDefinition(
    code: 'U0100',
    component: DtcComponent.network,
    name: 'Perda de comunicação com a ECM',
    severity: DtcSeverity.high,
  ),
  DtcDefinition(
    code: 'U0155',
    component: DtcComponent.network,
    name: 'Perda de comunicação com o painel',
    severity: DtcSeverity.medium,
  ),
];
