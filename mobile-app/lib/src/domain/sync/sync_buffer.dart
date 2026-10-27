import 'dart:collection';

import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';

/// Bloco guardado no buffer, com o instante (monotônico) da leitura.
@immutable
class BufferedBlock {
  /// Cria o bloco guardado.
  const BufferedBlock({required this.block, required this.readAt});

  /// Bloco lido do dongle.
  final SyncBlock block;

  /// Instante monotônico em que entrou no buffer.
  final Duration readAt;
}

class _Entry {
  _Entry(this.block);

  final BufferedBlock block;
  bool inFlight = false;
}

/// Buffer em memória dos blocos lidos e ainda não confirmados, ordenado por
/// `seq`. Blocos marcadores ocupam uma vaga como qualquer outro.
class SyncBuffer {
  /// Cria o buffer com [capacity] vagas (`B_max`).
  SyncBuffer({required this.capacity});

  /// Capacidade (`B_max`); reduzir não descarta o que já está guardado.
  int capacity;

  final SplayTreeMap<int, _Entry> _entries = SplayTreeMap();

  /// Blocos guardados (pendentes e em POST).
  int get length => _entries.length;

  /// Vagas livres.
  int get freeSlots => capacity > length ? capacity - length : 0;

  /// Blocos em POST.
  int get inFlightCount => _entries.values.where((e) => e.inFlight).length;

  /// Blocos fora de POST.
  int get pendingCount => length - inFlightCount;

  /// Mais antigo bloco fora de POST, ou `null`.
  BufferedBlock? get oldestPending {
    for (final e in _entries.values) {
      if (!e.inFlight) return e.block;
    }
    return null;
  }

  /// Guarda [b]; `false` se o `seq` já existe (duplicata) ou se está cheio.
  bool add(BufferedBlock b) {
    if (_entries.containsKey(b.block.seq) || length >= capacity) return false;
    _entries[b.block.seq] = _Entry(b);
    return true;
  }

  /// Os [max] blocos pendentes mais antigos, em ordem, marcados em POST.
  List<BufferedBlock> takeForPost(int max) {
    final out = <BufferedBlock>[];
    for (final e in _entries.values) {
      if (out.length >= max) break;
      if (e.inFlight) continue;
      e.inFlight = true;
      out.add(e.block);
    }
    return out;
  }

  /// O POST falhou: os blocos em voo voltam a pendentes.
  void releaseInFlight() {
    for (final e in _entries.values) {
      e.inFlight = false;
    }
  }

  /// Remove os blocos com `seq <= ackSeq` (em voo ou não).
  void removeUpTo(int ackSeq) {
    _entries.removeWhere((seq, _) => seq <= ackSeq);
  }

  /// Descarta tudo o que não está em POST.
  void dropPending() {
    _entries.removeWhere((_, e) => !e.inFlight);
  }
}
