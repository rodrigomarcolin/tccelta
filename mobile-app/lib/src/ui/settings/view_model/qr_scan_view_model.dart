import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/crypto/psk_cipher.dart';
import 'package:tccelta_mobile/src/domain/repositories/camera_permissions_repository.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';

/// Fase da permissão de câmera no fluxo de leitura de QR code da PSK.
enum QrPermissionPhase {
  /// Verificando se a permissão já foi concedida (estado inicial, silencioso).
  checking,

  /// Ainda não pedido.
  idle,

  /// Negado — mostrar orientação/repetir.
  denied,

  /// Negado permanentemente — só os ajustes do app resolvem.
  permanentlyDenied,

  /// Concedido — pode mostrar a câmera.
  granted,
}

/// Estado do fluxo de leitura de QR code da PSK: a fase da permissão de
/// câmera + o resultado (ou erro) da decodificação.
class QrScanState {
  /// Cria o estado.
  const QrScanState({
    this.permission = QrPermissionPhase.checking,
    this.errorMessage,
    this.resultHex,
  });

  /// Fase da permissão de câmera.
  final QrPermissionPhase permission;

  /// Feedback transitório de QR inválido (não é a chave, nunca ecoa o texto
  /// lido — ver nota de segurança em [QrScanViewModel.onDetected]).
  final String? errorMessage;

  /// Preenchido quando um frame decodifica uma PSK válida — a tela reage a
  /// isso fechando e devolvendo o valor.
  final String? resultHex;

  /// Cópia com campos sobrescritos. `clearError: true` zera [errorMessage]
  /// mesmo passando `null` (que por si só significa "sem mudança").
  QrScanState copyWith({
    QrPermissionPhase? permission,
    String? errorMessage,
    bool clearError = false,
    String? resultHex,
  }) => QrScanState(
    permission: permission ?? this.permission,
    errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
    resultHex: resultHex ?? this.resultHex,
  );
}

/// ViewModel do fluxo de leitura de QR code da PSK.
///
/// Não importa `package:mobile_scanner` — só conhece a fase da permissão de
/// câmera e a string bruta que a tela extrai de cada frame decodificado. É
/// esse isolamento que torna a lógica testável sem câmera real (ver
/// `qr_scan_view_model_test.dart`).
class QrScanViewModel extends Notifier<QrScanState> {
  CameraPermissionsRepository get _repo =>
      ref.read(cameraPermissionsRepositoryProvider);

  DateTime? _lastInvalidAt;

  @override
  QrScanState build() {
    unawaited(_check());
    return const QrScanState();
  }

  Future<void> _check() async {
    final has = await _repo.hasCameraPermission();
    state = state.copyWith(
      permission: has ? QrPermissionPhase.granted : QrPermissionPhase.idle,
    );
  }

  /// Pede acesso à câmera. Se negado, distingue negação simples de negação
  /// permanente (a UI troca "Permitir" por "Abrir ajustes do app" nesse
  /// segundo caso).
  Future<void> request() async {
    final granted = await _repo.requestCameraPermission();
    if (granted) {
      state = state.copyWith(permission: QrPermissionPhase.granted);
      return;
    }
    final locked = await _repo.isPermanentlyDenied();
    state = state.copyWith(
      permission: locked
          ? QrPermissionPhase.permanentlyDenied
          : QrPermissionPhase.denied,
    );
  }

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
/// entrada em `/psk-setup/scan-qr` a permissão é re-checada do zero.
final NotifierProvider<QrScanViewModel, QrScanState> qrScanViewModelProvider =
    NotifierProvider<QrScanViewModel, QrScanState>(
      QrScanViewModel.new,
      isAutoDispose: true,
    );
