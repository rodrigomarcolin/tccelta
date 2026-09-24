import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/crypto/psk_cipher.dart';

/// Estado do fluxo de leitura de QR code da PSK: o resultado (ou erro) da
/// decodificação. A permissão de câmera já foi resolvida por
/// `CameraPermissionsScreen` antes desta tela ser alcançada — este estado não
/// lida com permissão nenhuma.
class QrScanState {
  /// Cria o estado.
  const QrScanState({this.errorMessage, this.resultHex});

  /// Feedback transitório de QR inválido (não é a chave, nunca ecoa o texto
  /// lido — ver nota de segurança em [QrScanViewModel.onDetected]).
  final String? errorMessage;

  /// Preenchido quando um frame decodifica uma PSK válida — a tela reage a
  /// isso fechando e devolvendo o valor.
  final String? resultHex;

  /// Cópia com campos sobrescritos. `clearError: true` zera [errorMessage]
  /// mesmo passando `null` (que por si só significa "sem mudança").
  QrScanState copyWith({
    String? errorMessage,
    bool clearError = false,
    String? resultHex,
  }) => QrScanState(
    errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
    resultHex: resultHex ?? this.resultHex,
  );
}

/// ViewModel do fluxo de leitura de QR code da PSK.
///
/// Não importa `package:mobile_scanner` — só conhece a string bruta que a
/// tela extrai de cada frame decodificado. É esse isolamento que torna a
/// lógica testável sem câmera real (ver `qr_scan_view_model_test.dart`).
class QrScanViewModel extends Notifier<QrScanState> {
  DateTime? _lastInvalidAt;

  @override
  QrScanState build() => const QrScanState();

  /// Processa o texto bruto decodificado de um frame de câmera.
  ///
  /// Nunca loga [rawValue] — só compara contra a regra de formato da PSK. Um
  /// QR inválido gera uma mensagem estática de erro (nunca ecoa o conteúdo
  /// lido) com debounce de 1.5s para não empilhar feedback a cada frame
  /// enquanto o usuário mira num QR errado. Uma vez resolvido (`resultHex`
  /// setado), frames adicionais são ignorados.
  void onDetected(String? rawValue) {
    if (state.resultHex != null) return;
    final value = rawValue?.trim() ?? '';
    if (!PskCipher.isValidHex(value)) {
      final now = DateTime.now();
      if (_lastInvalidAt != null &&
          now.difference(_lastInvalidAt!) <
              const Duration(milliseconds: 1500)) {
        return;
      }
      _lastInvalidAt = now;
      state = state.copyWith(
        errorMessage:
            'QR code não contém uma chave PSK válida (64 caracteres hex).',
      );
      return;
    }
    state = state.copyWith(resultHex: value, clearError: true);
  }
}

/// Provider do [QrScanViewModel].
///
/// `autoDispose`: o estado não deve sobreviver à presença da tela — a cada
/// entrada em `/psk-setup/scan-qr` o resultado/erro de uma leitura anterior é
/// zerado.
final NotifierProvider<QrScanViewModel, QrScanState> qrScanViewModelProvider =
    NotifierProvider<QrScanViewModel, QrScanState>(
      QrScanViewModel.new,
      isAutoDispose: true,
    );
