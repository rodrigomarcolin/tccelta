import 'package:flutter/foundation.dart';

/// Reason why an ELM327 command was considered complete.
enum ElmCompletionReason { prompt, noData, rejected, malformed }

/// One ECU response extracted from one ELM327 response line.
@immutable
class ElmResponse {
  const ElmResponse({
    required this.service,
    required this.payload,
    required this.rawLine,
    this.pid,
    this.ecuId,
  });

  final int service;
  final int? pid;
  final int? ecuId;
  final List<int> payload;
  final String rawLine;

  @override
  String toString() =>
      'ElmResponse(${ecuId == null ? '?' : ecuId!.toRadixString(16)}, '
      '${service.toRadixString(16)}${pid == null ? '' : pid!.toRadixString(16)}, '
      '$payload)';
}

/// All structured responses produced by one serialized ELM327 command.
@immutable
class ElmResponseSet {
  const ElmResponseSet({
    required this.responses,
    required this.completion,
    required this.raw,
  });

  final List<ElmResponse> responses;
  final ElmCompletionReason completion;
  final String raw;
}
