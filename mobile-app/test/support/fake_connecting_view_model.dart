import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';

/// [ConnectingViewModel] falso: parte de um estado fixo, registra as ações
/// chamadas pela UI e deixa o teste emitir novos estados.
class FakeConnectingViewModel extends ConnectingViewModel {
  /// Cria o falso com o estado inicial [initial].
  FakeConnectingViewModel(this.initial);

  /// Estado inicial devolvido por `build`.
  final ConnectingState initial;

  /// Ações recebidas, na ordem (`retry`, `cancel`, `assumePaired`,
  /// `confirmForgotten`, `repair`).
  final List<String> calls = [];

  /// Estado aplicado ao fim de `repair`; `null` mantém o atual.
  ConnectingState? afterRepair;

  @override
  ConnectingState build() => initial;

  /// Emite [next] como novo estado.
  // ignore: use_setters_to_change_properties
  void emit(ConnectingState next) => state = next;

  @override
  Future<void> retry() async => calls.add('retry');

  @override
  Future<void> cancel() async => calls.add('cancel');

  @override
  Future<void> assumePaired() async => calls.add('assumePaired');

  @override
  void confirmForgotten() => calls.add('confirmForgotten');

  @override
  Future<void> repair() async {
    calls.add('repair');
    final next = afterRepair;
    if (next != null) state = next;
  }
}
