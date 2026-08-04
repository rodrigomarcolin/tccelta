import 'package:tccelta_mobile/src/data/datasources/permissions_datasource.dart';
import 'package:tccelta_mobile/src/domain/repositories/permissions_repository.dart';

/// Impl do [PermissionsRepository] — *source of truth* das permissões de BLE.
///
/// Hoje apenas delega ao [PermissionsDatasource]; é o ponto único caso no
/// futuro seja preciso cache ou conversão de erro em `Failure`.
class PermissionsRepositoryImpl implements PermissionsRepository {
  /// Cria o repository sobre um [PermissionsDatasource].
  const PermissionsRepositoryImpl(this._ds);

  final PermissionsDatasource _ds;

  @override
  Future<bool> hasBluetoothPermission() => _ds.hasBluetoothPermission();

  @override
  Future<bool> requestBluetoothPermission() => _ds.requestBluetoothPermission();
}
