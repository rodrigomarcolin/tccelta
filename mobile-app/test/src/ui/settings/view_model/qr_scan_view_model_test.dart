import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/ui/settings/view_model/qr_scan_view_model.dart';

const _validHex =
    'aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899';

void main() {
  ProviderContainer buildContainer() {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    // Mantém o provider `autoDispose` vivo durante o teste.
    container.listen<QrScanState>(
      qrScanViewModelProvider,
      (_, _) {},
      fireImmediately: true,
    );
    return container;
  }

  test('build inicia sem resultado nem erro', () {
    final container = buildContainer();
    final state = container.read(qrScanViewModelProvider);
    expect(state.resultHex, isNull);
    expect(state.errorMessage, isNull);
  });

  test('onDetected com hex válido preenche resultHex', () {
    final container = buildContainer();
    container.read(qrScanViewModelProvider.notifier).onDetected(_validHex);
    expect(container.read(qrScanViewModelProvider).resultHex, _validHex);
  });

  test('onDetected com lixo seta errorMessage e não preenche resultHex', () {
    final container = buildContainer();
    container.read(qrScanViewModelProvider.notifier).onDetected('lixo');
    final state = container.read(qrScanViewModelProvider);
    expect(state.resultHex, isNull);
    expect(state.errorMessage, isNotNull);
  });

  test('onDetected após já resolvido ignora frames adicionais', () {
    final container = buildContainer();
    container.read(qrScanViewModelProvider.notifier)
      ..onDetected(_validHex)
      ..onDetected(_validHex.replaceFirst('a', 'b'));
    expect(container.read(qrScanViewModelProvider).resultHex, _validHex);
  });
}
