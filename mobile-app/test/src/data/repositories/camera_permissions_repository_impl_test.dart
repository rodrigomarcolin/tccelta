import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:tccelta_mobile/src/data/datasources/camera_permissions_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/camera_permissions_repository_impl.dart';

class _MockCameraPermissionsDatasource extends Mock
    implements CameraPermissionsDatasource {}

void main() {
  late _MockCameraPermissionsDatasource ds;
  late CameraPermissionsRepositoryImpl repo;

  setUp(() {
    ds = _MockCameraPermissionsDatasource();
    repo = CameraPermissionsRepositoryImpl(ds);
  });

  test('hasCameraPermission delega ao datasource', () async {
    when(ds.hasCameraPermission).thenAnswer((_) async => true);
    expect(await repo.hasCameraPermission(), isTrue);
    verify(ds.hasCameraPermission).called(1);
  });

  test('requestCameraPermission delega ao datasource', () async {
    when(ds.requestCameraPermission).thenAnswer((_) async => false);
    expect(await repo.requestCameraPermission(), isFalse);
    verify(ds.requestCameraPermission).called(1);
  });

  test('isPermanentlyDenied delega ao datasource', () async {
    when(ds.isPermanentlyDenied).thenAnswer((_) async => true);
    expect(await repo.isPermanentlyDenied(), isTrue);
    verify(ds.isPermanentlyDenied).called(1);
  });
}
