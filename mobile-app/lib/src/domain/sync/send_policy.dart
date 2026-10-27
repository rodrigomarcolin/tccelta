import 'package:tccelta_mobile/src/domain/sync/sync_buffer.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// Regra de envio ao servidor: posta quando o bloco mais antigo atinge
/// `T_lat`, quando há blocos para um lote cheio ou com a flash alta.
class SendPolicy {
  /// Cria a regra com a latência máxima [tLat].
  const SendPolicy({required this.tLat, this.maxPerPost = kMaxBlocksPerPost});

  /// Tempo máximo de um bloco no buffer.
  final Duration tLat;

  /// Máximo de blocos por POST.
  final int maxPerPost;

  /// Quantos blocos postar agora (0 = não postar).
  int lotSize({
    required Duration now,
    required SyncBuffer buffer,
    required bool postInFlight,
    required bool flashHigh,
    required Duration? notBefore,
    required int currentLotLimit,
  }) {
    if (postInFlight) return 0;
    final oldest = buffer.oldestPending;
    if (oldest == null) return 0;
    if (notBefore != null && now < notBefore) return 0;
    final pending = buffer.pendingCount;
    final due =
        pending >= maxPerPost || flashHigh || now - oldest.readAt >= tLat;
    if (!due) return 0;
    var n = pending;
    if (currentLotLimit < n) n = currentLotLimit;
    if (maxPerPost < n) n = maxPerPost;
    return n;
  }
}
