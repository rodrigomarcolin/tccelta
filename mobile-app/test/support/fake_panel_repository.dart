import 'package:tccelta_mobile/src/domain/repositories/panel_repository.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel_collection.dart';

/// [PanelRepository] falso e em memória (sem `shared_preferences`) — cobre
/// tanto o load inicial (via o construtor) quanto o que foi salvo depois
/// (via [saveCalls]/[lastSaved]), para testes de view model/tela que só
/// precisam de um repository previsível.
class FakePanelRepository implements PanelRepository {
  /// Cria o fake, opcionalmente já com [initial] "salvo" (o que [load]
  /// devolverá antes de qualquer [save]).
  FakePanelRepository({PanelsState? initial}) : _stored = initial;

  PanelsState? _stored;

  /// Quantas vezes [save] foi chamado.
  int saveCalls = 0;

  /// Último estado passado a [save], se algum.
  PanelsState? get lastSaved => _stored;

  @override
  Future<PanelsState?> load() async => _stored;

  @override
  Future<void> save(PanelsState state) async {
    _stored = state;
    saveCalls++;
  }
}
