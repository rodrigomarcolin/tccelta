import 'dart:typed_data';

import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// Motivo de uma montagem malsucedida.
enum AssemblyErrorKind {
  /// O contador do fragmento não é o esperado.
  counterGap,

  /// Fragmento de continuação sem início.
  continuationWithoutStart,

  /// Novo início com uma montagem em curso.
  startWhileAssembling,

  /// `L` maior que o máximo aceito.
  tooLong,

  /// Mais bytes do que `3 + L`.
  overflow,

  /// Fragmentos parados além do limite.
  timeout,
}

/// Resultado de alimentar o [FragmentAssembler].
sealed class AssemblyOutcome {
  const AssemblyOutcome();
}

/// Mensagem ainda incompleta.
final class AssemblyPending extends AssemblyOutcome {
  /// Cria o resultado.
  const AssemblyPending();
}

/// Mensagem TLV completa.
final class AssemblyComplete extends AssemblyOutcome {
  /// Cria o resultado com o [tlv] remontado.
  const AssemblyComplete(this.tlv);

  /// Bytes `T | L | V` remontados.
  final Uint8List tlv;
}

/// Montagem descartada por [kind].
final class AssemblyError extends AssemblyOutcome {
  /// Cria o resultado de erro.
  const AssemblyError(this.kind);

  /// Motivo.
  final AssemblyErrorKind kind;
}

/// Remonta mensagens TLV do `SYNC_TX` a partir dos fragmentos (byte de
/// controle: bit 7 = primeiro fragmento, bits 6..0 = contador mod 128).
class FragmentAssembler {
  /// Cria o remontador.
  FragmentAssembler({
    this.maxValueLength = kMaxSyncValueLength,
    this.timeout = kFragmentTimeout,
  });

  /// Maior `L` aceito.
  final int maxValueLength;

  /// Tempo parado que torna a montagem um erro.
  final Duration timeout;

  final BytesBuilder _bytes = BytesBuilder(copy: false);
  bool _assembling = false;
  int _nextCounter = 0;
  Duration _lastAt = Duration.zero;

  /// `true` enquanto há uma mensagem em montagem.
  bool get assembling => _assembling;

  void _reset() {
    _bytes.clear();
    _assembling = false;
  }

  /// Consome um [fragment]; [now] é tempo monotônico. Pode devolver duas
  /// saídas (início durante montagem: erro e a nova mensagem em andamento).
  List<AssemblyOutcome> add(List<int> fragment, Duration now) {
    if (fragment.isEmpty) return const [];
    final control = fragment[0];
    final isStart = control & 0x80 != 0;
    final counter = control & 0x7F;
    final out = <AssemblyOutcome>[];

    if (isStart) {
      if (_assembling) {
        out.add(const AssemblyError(AssemblyErrorKind.startWhileAssembling));
        _reset();
      }
      if (counter != 0) {
        return [...out, const AssemblyError(AssemblyErrorKind.counterGap)];
      }
      _assembling = true;
      _nextCounter = 1;
    } else {
      if (!_assembling) {
        return const [
          AssemblyError(AssemblyErrorKind.continuationWithoutStart),
        ];
      }
      if (counter != _nextCounter) {
        _reset();
        return const [AssemblyError(AssemblyErrorKind.counterGap)];
      }
      _nextCounter = (_nextCounter + 1) & 0x7F;
    }

    _bytes.add(fragment.sublist(1));
    _lastAt = now;

    if (_bytes.length >= 3) {
      final bytes = _bytes.toBytes();
      final length = bytes[1] | (bytes[2] << 8);
      if (length > maxValueLength) {
        _reset();
        return [...out, const AssemblyError(AssemblyErrorKind.tooLong)];
      }
      final total = 3 + length;
      if (bytes.length > total) {
        _reset();
        return [...out, const AssemblyError(AssemblyErrorKind.overflow)];
      }
      if (bytes.length == total) {
        _reset();
        return [...out, AssemblyComplete(bytes)];
      }
    }
    return [...out, const AssemblyPending()];
  }

  /// Devolve [AssemblyError] (timeout) se a montagem está parada há
  /// [timeout] ou mais, descartando-a; senão `null`.
  AssemblyOutcome? checkTimeout(Duration now) {
    if (_assembling && now - _lastAt >= timeout) {
      _reset();
      return const AssemblyError(AssemblyErrorKind.timeout);
    }
    return null;
  }
}
