import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/data/datasources/dongle_datasource.dart';
import 'package:tccelta_mobile/src/data/datasources/permissions_datasource.dart';
import 'package:tccelta_mobile/src/data/datasources/secure_dongle_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/dongle_repository_impl.dart';
import 'package:tccelta_mobile/src/data/repositories/permissions_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/permissions_repository.dart';
import 'package:tccelta_mobile/src/infra/ble/ble_plus.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';

/// Port BLE -> adapter concreto (infra). Trocar de lib = trocar SÓ este
/// provider; nada acima sabe qual biblioteca está embaixo.
final Provider<BleService> bleServiceProvider = Provider<BleService>(
  (_) => FlutterBluePlusBleService(),
);

/// Datasource específico do dongle, sobre o [bleServiceProvider].
/// Usa [SecureDongleDatasource] para envolver automaticamente a conexão BLE com
/// [EncryptedBleConnection] quando uma PSK estiver configurada.
final Provider<DongleDatasource> dongleDatasourceProvider =
    Provider<DongleDatasource>(
      (ref) => SecureDongleDatasource(
        ref.read(bleServiceProvider),
        ref.read(settingsServiceProvider),
      ),
    );

/// Repository = *source of truth* da conexão. Mantém a conexão viva, então é um
/// `Provider` comum (NÃO autoDispose).
final Provider<DongleRepository> dongleRepositoryProvider =
    Provider<DongleRepository>(
      (ref) => DongleRepositoryImpl(ref.read(dongleDatasourceProvider)),
    );

/// Datasource das permissões de BLE, sobre o plugin `permission_handler`.
final Provider<PermissionsDatasource> permissionsDatasourceProvider =
    Provider<PermissionsDatasource>((_) => const PermissionsDatasource());

/// Repository = *source of truth* das permissões. O view model fala com este
/// provider, nunca com o datasource.
final Provider<PermissionsRepository> permissionsRepositoryProvider =
    Provider<PermissionsRepository>(
      (ref) =>
          PermissionsRepositoryImpl(ref.read(permissionsDatasourceProvider)),
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
