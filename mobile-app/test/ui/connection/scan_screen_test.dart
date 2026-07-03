import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view/scan_screen.dart';

import '../../support/fake_ble_service.dart';

void main() {
  Widget wrap(FakeBleService fake) => ProviderScope(
        overrides: [bleServiceProvider.overrideWithValue(fake)],
        child: MaterialApp(theme: AppTheme.dark, home: const ScanScreen()),
      );

  testWidgets('lista os dongles encontrados', (tester) async {
    await tester.pumpWidget(
      wrap(
        FakeBleService(
          devices: const [BleDevice(id: '1', name: 'OBD2Dongle', rssi: -50)],
        ),
      ),
    );
    // Um pump executa o addPostFrameCallback (startScan); o segundo deixa a
    // stream emitir os resultados.
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.text('OBD2Dongle'), findsOneWidget);
  });

  testWidgets('mostra o MAC quando o dongle não anuncia nome', (tester) async {
    await tester.pumpWidget(
      wrap(
        FakeBleService(
          devices: const [
            BleDevice(id: '06:E5:28:3B:FD:E0', name: '', rssi: -50),
          ],
        ),
      ),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.text('06:E5:28:3B:FD:E0'), findsOneWidget);
  });

  testWidgets('omite o dongle sem nome e sem id (HEX)', (tester) async {
    await tester.pumpWidget(
      wrap(
        FakeBleService(
          devices: const [BleDevice(id: '', name: '', rssi: -50)],
        ),
      ),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.text('Nenhum dongle encontrado ainda…'), findsOneWidget);
  });

  testWidgets('mostra estado vazio quando nada é encontrado', (tester) async {
    await tester.pumpWidget(wrap(FakeBleService()));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.text('Nenhum dongle encontrado ainda…'), findsOneWidget);
  });

  testWidgets('reencontra o dongle ao tocar em Procurar novamente',
      (tester) async {
    final fake = FakeBleService(
      devices: const [BleDevice(id: '1', name: 'OBD2Dongle', rssi: -50)],
    );
    await tester.pumpWidget(wrap(fake));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));
    expect(find.text('OBD2Dongle'), findsOneWidget);

    // O scan inicial terminou (fake de disparo único); tocar em "Procurar
    // novamente" reinicia a busca e reencontra o dongle.
    await tester.tap(find.text('Procurar novamente'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.text('OBD2Dongle'), findsOneWidget);
    expect(fake.scanCount, 2);
  });
}
