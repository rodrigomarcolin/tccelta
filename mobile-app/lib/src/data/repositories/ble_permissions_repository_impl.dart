import 'package:tccelta_mobile/src/data/datasources/ble_permissions_datasource.dart';
import 'package:tccelta_mobile/src/domain/repositories/ble_permissions_repository.dart';

/// Impl do [BlePermissionsRepository] — *source of truth* das permissões de
/// BLE.
///
/// Hoje apenas delega ao [BlePermissionsDatasource]; é o ponto único caso no
/// futuro seja preciso cache ou conversão de erro em `Failure`.
class BlePermissionsRepositoryImpl implements BlePermissionsRepository {
  /// Cria o repository sobre um [BlePermissionsDatasource].
  const BlePermissionsRepositoryImpl(this._ds);

  final BlePermissionsDatasource _ds;

  @override
  Future<bool> hasBluetoothPermission() => _ds.hasBluetoothPermission();

  @override
  Future<bool> requestBluetoothPermission() => _ds.requestBluetoothPermission();
}
