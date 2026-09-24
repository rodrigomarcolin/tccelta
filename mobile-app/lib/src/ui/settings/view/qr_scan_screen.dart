import 'dart:async';

import 'package:app_settings/app_settings.dart';
import 'package:flutter/material.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:go_router/go_router.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/settings/view_model/qr_scan_view_model.dart';

/// Leitura do QR code exibido/impresso no dongle com a chave PSK (64
/// caracteres hex).
///
/// Empilhada por `PskSettingsScreen` via `context.push<String>`: devolve o
/// hex lido através de `context.pop`, nunca salva a PSK sozinha — quem
/// confirma "Continuar e salvar chave" é sempre a tela de origem.
class QrScanScreen extends HookConsumerWidget {
  /// Cria a tela de leitura de QR code.
  const QrScanScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final controller = useMemoized(
      () => MobileScannerController(formats: const [BarcodeFormat.qrCode]),
    );
    useEffect(
      () =>
          () => unawaited(controller.dispose()),
      const [],
    );

    ref.listen<QrScanState>(qrScanViewModelProvider, (previous, next) {
      if (next.resultHex != null) {
        unawaited(controller.stop());
        context.pop(next.resultHex);
        return;
      }
      if (next.errorMessage != null &&
          next.errorMessage != previous?.errorMessage) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(next.errorMessage!)));
      }
    });

    final state = ref.watch(qrScanViewModelProvider);

    return switch (state.permission) {
      QrPermissionPhase.checking => const ConnectionBackground(
        child: Center(
          child: CircularProgressIndicator(color: AppColors.cyan500),
        ),
      ),
      QrPermissionPhase.granted => _ScannerView(controller: controller),
      QrPermissionPhase.idle ||
      QrPermissionPhase.denied ||
      QrPermissionPhase.permanentlyDenied => _PermissionRequest(
        locked: state.permission == QrPermissionPhase.permanentlyDenied,
        onRequest: () => ref.read(qrScanViewModelProvider.notifier).request(),
      ),
    };
  }
}

/// UI de pedido de permissão de câmera — mesma composição
/// (`ConnectionStateView`) usada pelo pedido de permissão de Bluetooth.
class _PermissionRequest extends StatelessWidget {
  const _PermissionRequest({required this.locked, required this.onRequest});

  /// `true` quando a permissão foi negada permanentemente: o diálogo do
  /// sistema não aparece mais, só os ajustes do app resolvem.
  final bool locked;

  final VoidCallback onRequest;

  Future<void> _openAppSettings(BuildContext context) async {
    try {
      await AppSettings.openAppSettings(asAnotherTask: true);
    } on Object {
      if (!context.mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Abra os ajustes do app.')),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return ConnectionStateView(
      tone: StatusTone.warning,
      icon: AppIconData.cadeado,
      title: 'Permitir câmera',
      description:
          'O app usa a câmera só para ler o QR code com a chave PSK do '
          'dongle.',
      primaryAction: locked
          ? AppButton(
              variant: AppButtonVariant.warning,
              onPressed: () => _openAppSettings(context),
              child: const Text('Abrir ajustes do app'),
            )
          : AppButton(
              onPressed: onRequest,
              child: const Text('Permitir'),
            ),
      secondaryAction: AppButton(
        variant: AppButtonVariant.link,
        onPressed: () => context.pop(),
        child: const Text('Digitar a chave manualmente'),
      ),
    );
  }
}

/// Câmera ao vivo + viewfinder, montada só quando a permissão está concedida.
class _ScannerView extends HookConsumerWidget {
  const _ScannerView({required this.controller});

  final MobileScannerController controller;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        backgroundColor: AppColors.bgScreen,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_ios_new, size: 18),
          color: AppColors.textSecondary,
          onPressed: () => context.pop(),
        ),
        title: Text(
          'Escanear QR code',
          style: AppTypography.ui(
            const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary,
            ),
          ),
        ),
        centerTitle: false,
        actions: [
          ValueListenableBuilder<MobileScannerState>(
            valueListenable: controller,
            builder: (context, value, _) {
              final torch = value.torchState;
              final available = torch != TorchState.unavailable;
              return IconButton(
                icon: Icon(
                  torch == TorchState.on
                      ? Icons.flash_on_rounded
                      : Icons.flash_off_rounded,
                  size: 20,
                ),
                color: AppColors.textSecondary,
                tooltip: 'Lanterna',
                onPressed: available
                    ? () => unawaited(controller.toggleTorch())
                    : null,
              );
            },
          ),
        ],
      ),
      body: Stack(
        fit: StackFit.expand,
        children: [
          MobileScanner(
            controller: controller,
            onDetect: (capture) => ref
                .read(qrScanViewModelProvider.notifier)
                .onDetected(capture.barcodes.firstOrNull?.rawValue),
            errorBuilder: (context, error) => const _CameraErrorView(),
          ),
          const _Viewfinder(),
          Positioned(
            left: AppSpacing.s7,
            right: AppSpacing.s7,
            bottom: AppSpacing.s9,
            child: Text(
              'Aponte a câmera para o QR code da chave PSK',
              textAlign: TextAlign.center,
              style: AppTypography.ui(
                const TextStyle(fontSize: 13, color: Colors.white),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Moldura simples indicando a área de leitura — o `mobile_scanner` não
/// recorta a busca a essa área (recorte fica a cargo de [Barcode], não do
/// preview), então é só orientação visual.
class _Viewfinder extends StatelessWidget {
  const _Viewfinder();

  @override
  Widget build(BuildContext context) {
    return IgnorePointer(
      child: Center(
        child: Container(
          width: 240,
          height: 240,
          decoration: BoxDecoration(
            border: Border.all(color: AppColors.cyan500, width: 2),
            borderRadius: BorderRadius.circular(16),
          ),
        ),
      ),
    );
  }
}

/// Cobre "sem câmera disponível" — cai de volta pro fluxo digitar/colar via
/// "Voltar", nunca um beco sem saída.
class _CameraErrorView extends StatelessWidget {
  const _CameraErrorView();

  @override
  Widget build(BuildContext context) {
    return ColoredBox(
      color: AppColors.bgScreen,
      child: Center(
        child: Padding(
          padding: const EdgeInsets.all(AppSpacing.s7),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(
                Icons.videocam_off_rounded,
                size: 40,
                color: AppColors.textTertiary,
              ),
              const SizedBox(height: AppSpacing.s5),
              Text(
                'Câmera indisponível neste aparelho.',
                textAlign: TextAlign.center,
                style: AppTypography.ui(
                  const TextStyle(
                    fontSize: 13,
                    color: AppColors.textSecondary,
                  ),
                ),
              ),
              const SizedBox(height: AppSpacing.s7),
              AppButton(
                variant: AppButtonVariant.secondary,
                onPressed: () => context.pop(),
                child: const Text('Voltar'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
