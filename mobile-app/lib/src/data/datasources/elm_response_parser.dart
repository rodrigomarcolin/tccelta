import 'package:tccelta_mobile/src/domain/obd2/elm_response.dart';

/// Parses ELM327 lines while keeping ECU identity and payload boundaries.
///
/// It accepts both headerless output (`41 0C ...`) and header-prefixed output
/// (`7E8 06 41 0C ...`). The parser deliberately does not decode physical
/// values; that remains the responsibility of the OBD domain.
class ElmResponseParser {
  const ElmResponseParser._();

  /// Parses [raw] ELM327 output looking for responses to [responseService]
  /// (and [pid], when the service carries one).
  static ElmResponseSet parse(
    String raw, {
    required int responseService,
    int? pid,
  }) {
    final responses = <ElmResponse>[];
    for (final line in _lines(raw)) {
      final tokens = line
          .trim()
          .toUpperCase()
          .split(RegExp(r'\s+'))
          .where((token) => RegExp(r'^[0-9A-F]{2,8}$').hasMatch(token))
          .toList();
      if (tokens.isEmpty) continue;

      final marker = _findMarker(tokens, responseService, pid);
      if (marker == null) continue;
      final ecuId = _ecuIdBefore(tokens, marker.index);
      final payloadStart = marker.index + (pid == null ? 1 : 2);
      if (payloadStart > tokens.length) continue;
      final payload = <int>[];
      for (final token in tokens.skip(payloadStart)) {
        if (token.length != 2) continue;
        payload.add(int.parse(token, radix: 16));
      }
      responses.add(
        ElmResponse(
          service: responseService,
          pid: pid,
          ecuId: ecuId,
          payload: payload,
          rawLine: line.trim(),
        ),
      );
    }

    final upper = raw.toUpperCase();
    final completion = responses.isNotEmpty
        ? ElmCompletionReason.prompt
        : upper.contains('NO DATA') || upper.contains('NODATA')
        ? ElmCompletionReason.noData
        : upper.contains('?')
        ? ElmCompletionReason.rejected
        : ElmCompletionReason.malformed;
    return ElmResponseSet(
      responses: List.unmodifiable(responses),
      completion: completion,
      raw: raw,
    );
  }

  static Iterable<String> _lines(String raw) => raw
      .replaceAll('\r\n', '\n')
      .replaceAll('\r', '\n')
      .split('\n')
      .map((line) => line.trim())
      .where((line) => line.isNotEmpty);

  static _Marker? _findMarker(
    List<String> tokens,
    int service,
    int? pid,
  ) {
    final serviceHex = _hex(service);
    final pidHex = pid == null ? null : _hex(pid);
    for (var i = 0; i < tokens.length; i++) {
      if (tokens[i] != serviceHex) continue;
      if (pidHex == null || i + 1 < tokens.length && tokens[i + 1] == pidHex) {
        return _Marker(i);
      }
    }
    return null;
  }

  static int? _ecuIdBefore(List<String> tokens, int markerIndex) {
    if (markerIndex == 0) return null;
    final candidate = tokens[markerIndex - 1];
    if (candidate.length == 3 || candidate.length == 8) {
      return int.tryParse(candidate, radix: 16);
    }
    // Headered ELM output normally contains a CAN data-length byte between
    // the ECU identifier and the ISO-TP/OBD service marker.
    if (candidate.length == 2 && markerIndex >= 2) {
      final header = tokens[markerIndex - 2];
      if (header.length == 3 || header.length == 8) {
        return int.tryParse(header, radix: 16);
      }
    }
    return null;
  }

  static String _hex(int value) =>
      value.toRadixString(16).toUpperCase().padLeft(2, '0');
}

class _Marker {
  const _Marker(this.index);
  final int index;
}
