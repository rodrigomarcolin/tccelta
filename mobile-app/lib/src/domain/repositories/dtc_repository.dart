import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';

/// Contrato de leitura de diagnóstico (DTCs + luz de injeção).
///
/// É a *source of truth* dos códigos para a UI: fala Modos 03/07/0A (+ 02
/// para congelamento) sobre a `BleConnection` viva e lança `DtcReadFailure`
/// em erro. A camada de UI (view_model) nunca toca no transporte de bytes.
//
// Convenção do projeto: toda feature tem seu par contrato (domain) + impl
// (data), mesmo com um único método hoje — cresce sem quebrar a assinatura
// pública quando a leitura real (Modo 03/07/0A/02) precisar de mais métodos.
// ignore: one_member_abstracts
abstract interface class DtcRepository {
  /// Lê o retrato atual de diagnóstico (catálogo de códigos + MIL). Lança
  /// `DtcReadFailure` se não houver conexão pronta ou a leitura falhar.
  Future<DtcSnapshot> read();
}
