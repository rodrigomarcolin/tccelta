import 'dart:async';

import 'package:tccelta_mobile/src/domain/repositories/sync_settings_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';

/// Repository de configurações em memória (sem ajuste às faixas, para os testes
/// poderem usar `B_max` pequeno).
class FakeSyncSettingsRepository implements SyncSettingsRepository {
  /// Cria o repository com as configurações iniciais.
  FakeSyncSettingsRepository([this._current = const SyncSettings()]);

  SyncSettings _current;
  final StreamController<SyncSettings> _ctrl =
      StreamController<SyncSettings>.broadcast();

  /// Quantas vezes [save] foi chamado.
  int saves = 0;

  @override
  SyncSettings get current => _current;

  @override
  Stream<SyncSettings> watch() {
    late StreamController<SyncSettings> c;
    StreamSubscription<SyncSettings>? sub;
    c = StreamController<SyncSettings>(
      onListen: () {
        c.add(_current);
        sub = _ctrl.stream.listen(c.add);
      },
      onCancel: () => sub?.cancel(),
    );
    return c.stream;
  }

  @override
  Future<void> save(SyncSettings settings) async {
    saves++;
    _current = settings;
    _ctrl.add(settings);
  }
}
