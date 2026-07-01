import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/data/datasources/dongle_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/dongle_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/infra/ble/ble_plus.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';

/// Port BLE -> adapter concreto (infra). Trocar de lib = trocar SÓ este
/// provider; nada acima sabe qual biblioteca está embaixo.
final Provider<BleService> bleServiceProvider =
    Provider<BleService>((_) => FlutterBluePlusBleService());

/// Datasource específico do dongle, sobre o [bleServiceProvider].
final Provider<DongleDatasource> dongleDatasourceProvider =
    Provider<DongleDatasource>(
  (ref) => DongleDatasource(ref.read(bleServiceProvider)),
);

/// Repository = *source of truth* da conexão. Mantém a conexão viva, então é um
/// `Provider` comum (NÃO autoDispose).
final Provider<DongleRepository> dongleRepositoryProvider =
    Provider<DongleRepository>(
  (ref) => DongleRepositoryImpl(ref.read(dongleDatasourceProvider)),
);

/// Estado do adaptador Bluetooth do telefone (para gating/auto-avanço nas
/// telas de permissões e BT desligado).
final StreamProvider<BleAdapterState> adapterStateProvider =
    StreamProvider<BleAdapterState>(
  (ref) => ref.watch(dongleRepositoryProvider).adapterState,
);

/// Dongle escolhido na busca, lido pela tela de conexão (passa o device
/// scan -> connecting sem parâmetro de rota).
final NotifierProvider<SelectedDongle, BleDevice?> selectedDongleProvider =
    NotifierProvider<SelectedDongle, BleDevice?>(SelectedDongle.new);

/// Notifier do dongle selecionado.
class SelectedDongle extends Notifier<BleDevice?> {
  @override
  BleDevice? build() => null;

  /// Seleciona um [device] (ao tocar na busca).
  // ignore: use_setters_to_change_properties
  void select(BleDevice device) => state = device;

  /// Limpa a seleção (ex.: "Esquecer dispositivo").
  void clear() => state = null;
}
