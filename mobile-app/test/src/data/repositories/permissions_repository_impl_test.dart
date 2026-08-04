import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:tccelta_mobile/src/data/datasources/permissions_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/permissions_repository_impl.dart';

class _MockPermissionsDatasource extends Mock
    implements PermissionsDatasource {}

void main() {
  late _MockPermissionsDatasource ds;
  late PermissionsRepositoryImpl repo;

  setUp(() {
    ds = _MockPermissionsDatasource();
    repo = PermissionsRepositoryImpl(ds);
  });

  test('hasBluetoothPermission delega ao datasource', () async {
    when(ds.hasBluetoothPermission).thenAnswer((_) async => true);
    expect(await repo.hasBluetoothPermission(), isTrue);
    verify(ds.hasBluetoothPermission).called(1);
  });

  test('requestBluetoothPermission delega ao datasource', () async {
    when(ds.requestBluetoothPermission).thenAnswer((_) async => false);
    expect(await repo.requestBluetoothPermission(), isFalse);
    verify(ds.requestBluetoothPermission).called(1);
  });
}
