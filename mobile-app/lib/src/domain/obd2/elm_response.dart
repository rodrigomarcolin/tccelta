import 'package:flutter/foundation.dart';

/// Reason why an ELM327 command was considered complete.
enum ElmCompletionReason {
  /// Terminated by the ELM327 `>` prompt with at least one response parsed.
  prompt,

  /// The dongle/ECU replied `NO DATA`.
  noData,

  /// The dongle rejected the command (`?`).
  rejected,

  /// Completed, but no response line matched the expected service/PID.
  malformed,
}

/// One ECU response extracted from one ELM327 response line.
@immutable
class ElmResponse {
  /// Creates a parsed ECU response.
  const ElmResponse({
    required this.service,
    required this.payload,
    required this.rawLine,
    this.pid,
    this.ecuId,
  });

  /// OBD-II service (mode) this response answers, e.g. `0x01`.
  final int service;

  /// PID this response answers, when the service carries one.
  final int? pid;

  /// CAN response identifier, when the line included a header.
  final int? ecuId;

  /// Decoded response bytes, after the service/PID marker.
  final List<int> payload;

  /// The original ELM327 line this response was parsed from.
  final String rawLine;

  @override
  String toString() {
    final ecu = ecuId == null ? '?' : ecuId!.toRadixString(16);
    final pidHex = pid == null ? '' : pid!.toRadixString(16);
    return 'ElmResponse($ecu, ${service.toRadixString(16)}$pidHex, $payload)';
  }
}

/// All structured responses produced by one serialized ELM327 command.
@immutable
class ElmResponseSet {
  /// Creates a set of responses parsed from one ELM327 command's output.
  const ElmResponseSet({
    required this.responses,
    required this.completion,
    required this.raw,
  });

  /// Responses extracted from [raw], one per matching ECU/line.
  final List<ElmResponse> responses;

  /// Why the ELM327 command was considered complete.
  final ElmCompletionReason completion;

  /// The full, unparsed ELM327 output.
  final String raw;
}
