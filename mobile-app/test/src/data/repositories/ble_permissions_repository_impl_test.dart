import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:tccelta_mobile/src/data/datasources/ble_permissions_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/ble_permissions_repository_impl.dart';

class _MockBlePermissionsDatasource extends Mock
    implements BlePermissionsDatasource {}

void main() {
  late _MockBlePermissionsDatasource ds;
  late BlePermissionsRepositoryImpl repo;

  setUp(() {
    ds = _MockBlePermissionsDatasource();
    repo = BlePermissionsRepositoryImpl(ds);
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
