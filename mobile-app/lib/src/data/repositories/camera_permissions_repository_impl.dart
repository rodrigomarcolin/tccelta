import 'package:tccelta_mobile/src/data/datasources/camera_permissions_datasource.dart';
import 'package:tccelta_mobile/src/domain/repositories/camera_permissions_repository.dart';

/// Impl do [CameraPermissionsRepository] — *source of truth* da permissão de
/// câmera.
///
/// Hoje apenas delega ao [CameraPermissionsDatasource]; é o ponto único caso
/// no futuro seja preciso cache ou conversão de erro em `Failure`.
class CameraPermissionsRepositoryImpl implements CameraPermissionsRepository {
  /// Cria o repository sobre um [CameraPermissionsDatasource].
  const CameraPermissionsRepositoryImpl(this._ds);

  final CameraPermissionsDatasource _ds;

  @override
  Future<bool> hasCameraPermission() => _ds.hasCameraPermission();

  @override
  Future<bool> requestCameraPermission() => _ds.requestCameraPermission();

  @override
  Future<bool> isPermanentlyDenied() => _ds.isPermanentlyDenied();
}
