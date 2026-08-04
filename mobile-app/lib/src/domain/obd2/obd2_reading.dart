import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

/// Uma leitura decodificada de um [Obd2Pid] — o par PID + valor físico.
///
/// Modelo de domínio PURO e imutável: o [value] já vem convertido pela fórmula
/// (ver [Obd2Pid.decode]); a formatação/arredondamento para exibição é da UI.
@immutable
class Obd2Reading {
  /// Cria a leitura de [pid] com o [value] já em grandeza física.
  const Obd2Reading({required this.pid, required this.value});

  /// PID de origem (carrega rótulo e unidade).
  final Obd2Pid pid;

  /// Valor convertido na unidade do PID (ex.: 1500.0 para RPM).
  final double value;

  @override
  bool operator ==(Object other) =>
      other is Obd2Reading && other.pid == pid && other.value == value;

  @override
  int get hashCode => Object.hash(pid, value);

  @override
  String toString() => 'Obd2Reading(${pid.command}: $value ${pid.unit})';
}
