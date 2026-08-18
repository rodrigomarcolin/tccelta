import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:go_router/go_router.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';

/// Tela de configuração da chave pré-compartilhada (PSK) AES-256-GCM.
///
/// O usuário digita a chave de 64 chars hex (= 32 bytes). Enquanto não houver
/// chave configurada, a comunicação BLE usa texto puro (modo compatível). Ao
/// salvar uma chave válida, toda nova conexão passa por [EncryptedBleConnection].
///
/// Quando [setupFlow] é `true`, a tela funciona como um passo do fluxo de
/// conexão (aberta pela `ScanScreen` ao selecionar um dongle): os botões viram
/// "Salvar chave" / "Continuar sem chave" e, após qualquer um dos dois, segue
/// para [AppRoutes.connecting]. Quando `false` (padrão — aberta pela aba
/// "Mais" ou pela `ConnectedScreen`), a tela só gerencia a chave localmente,
/// sem navegar.
class PskSettingsScreen extends HookConsumerWidget {
  /// Cria a tela de configurações de criptografia.
  const PskSettingsScreen({this.setupFlow = false, super.key});

  /// `true` quando esta tela é um passo do fluxo de conexão (pré-conexão).
  final bool setupFlow;

  static const int _hexLen = 64;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final pskAsync = ref.watch(pskNotifierProvider);
    final controller = useTextEditingController();
    final formKey = useMemoized(GlobalKey<FormState>.new);
    final obscure = useState(true);
    final saving = useState(false);

    // Populate the field when the stored value loads.
    useEffect(
      () {
        pskAsync.whenData((v) {
          if (controller.text.isEmpty && v != null) {
            controller.text = v;
          }
        });
        return null;
      },
      [pskAsync],
    );

    final stored = pskAsync.asData?.value;
    final isActive = stored != null && stored.isNotEmpty;

    Future<void> onSave() async {
      if (!formKey.currentState!.validate()) return;
      saving.value = true;
      try {
        await ref
            .read(pskNotifierProvider.notifier)
            .save(controller.text.trim());
        if (!context.mounted) return;
        if (setupFlow) {
          unawaited(context.push(AppRoutes.connecting));
        }
      } finally {
        saving.value = false;
      }
    }

    Future<void> onClear() async {
      saving.value = true;
      try {
        await ref.read(pskNotifierProvider.notifier).clear();
        controller.clear();
        if (context.mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Chave removida — nova conexão usará texto puro.'),
            ),
          );
        }
      } finally {
        saving.value = false;
      }
    }

    Future<void> onContinueWithoutKey() async {
      saving.value = true;
      try {
        await ref.read(pskNotifierProvider.notifier).clear();
        if (context.mounted) unawaited(context.push(AppRoutes.connecting));
      } finally {
        saving.value = false;
      }
    }

    return Scaffold(
      backgroundColor: AppColors.bgScreen,
      appBar: AppBar(
        backgroundColor: AppColors.bgScreen,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_ios_new, size: 18),
          color: AppColors.textSecondary,
          onPressed: () => context.pop(),
        ),
        title: Text(
          'Criptografia',
          style: AppTypography.ui(
            const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary,
            ),
          ),
        ),
        centerTitle: false,
      ),
      body: ConnectionBackground(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: AppSpacing.s7),
          child: Form(
            key: formKey,
            child: ListView(
              padding: const EdgeInsets.symmetric(vertical: AppSpacing.s7),
              children: [
                // ── Status badge ─────────────────────────────────────────
                _StatusBanner(isActive: isActive),
                const SizedBox(height: AppSpacing.s7),

                // ── Section label ─────────────────────────────────────────
                Text(
                  'CHAVE PRÉ-COMPARTILHADA (PSK)',
                  style: AppTypography.overline,
                ),
                const SizedBox(height: AppSpacing.s4),

                // ── Key input ─────────────────────────────────────────────
                TextFormField(
                  controller: controller,
                  obscureText: obscure.value,
                  style: AppTypography.mono(
                    const TextStyle(
                      fontSize: 13,
                      color: AppColors.textPrimary,
                      letterSpacing: 1.1,
                    ),
                  ),
                  decoration: InputDecoration(
                    hintText: 'Cole os 64 caracteres hex aqui…',
                    hintStyle: AppTypography.mono(
                      const TextStyle(
                        fontSize: 13,
                        color: AppColors.textTertiary,
                      ),
                    ),
                    filled: true,
                    fillColor: AppColors.surfaceCard,
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: AppSpacing.s5,
                      vertical: AppSpacing.s4,
                    ),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: const BorderSide(
                        color: AppColors.borderHairline,
                      ),
                    ),
                    enabledBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: const BorderSide(
                        color: AppColors.borderHairline,
                      ),
                    ),
                    focusedBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: const BorderSide(
                        color: AppColors.accent,
                        width: 1.5,
                      ),
                    ),
                    errorBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: const BorderSide(color: AppColors.danger),
                    ),
                    focusedErrorBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(8),
                      borderSide: const BorderSide(color: AppColors.danger),
                    ),
                    suffixIcon: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        // Paste button
                        IconButton(
                          icon: const Icon(
                            Icons.content_paste_rounded,
                            size: 18,
                          ),
                          color: AppColors.textSecondary,
                          tooltip: 'Colar',
                          onPressed: () async {
                            final data = await Clipboard.getData(
                              Clipboard.kTextPlain,
                            );
                            final text = data?.text?.trim();
                            if (text != null) controller.text = text;
                          },
                        ),
                        // Visibility toggle
                        IconButton(
                          icon: Icon(
                            obscure.value
                                ? Icons.visibility_off_outlined
                                : Icons.visibility_outlined,
                            size: 18,
                          ),
                          color: AppColors.textSecondary,
                          tooltip: obscure.value ? 'Mostrar' : 'Ocultar',
                          onPressed: () => obscure.value = !obscure.value,
                        ),
                      ],
                    ),
                  ),
                  inputFormatters: [
                    FilteringTextInputFormatter.allow(RegExp('[0-9a-fA-F]')),
                    LengthLimitingTextInputFormatter(_hexLen),
                  ],
                  validator: (v) {
                    final t = v?.trim() ?? '';
                    if (t.isEmpty) return null; // empty = clear intent
                    if (t.length != _hexLen) {
                      return 'A chave deve ter exatamente $_hexLen caracteres hex (${t.length}/$_hexLen)';
                    }
                    return null;
                  },
                ),
                const SizedBox(height: AppSpacing.s3),
                Text(
                  'AES-256-GCM · 64 caracteres hexadecimais = 32 bytes',
                  style: AppTypography.ui(
                    const TextStyle(
                      fontSize: 11,
                      color: AppColors.textTertiary,
                      fontWeight: FontWeight.w400,
                    ),
                  ),
                ),
                const SizedBox(height: AppSpacing.s7),

                // ── Actions ───────────────────────────────────────────────
                AppButton(
                  onPressed: saving.value ? null : onSave,
                  child: saving.value
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: AppColors.accentOn,
                          ),
                        )
                      : const Text('Continuar e salvar chave'),
                ),
                if (setupFlow) ...[
                  const SizedBox(height: AppSpacing.s3),
                  AppButton(
                    variant: AppButtonVariant.secondary,
                    onPressed: saving.value ? null : onContinueWithoutKey,
                    child: const Text('Continuar sem chave'),
                  ),
                ] else if (isActive) ...[
                  const SizedBox(height: AppSpacing.s3),
                  AppButton(
                    variant: AppButtonVariant.secondary,
                    onPressed: saving.value ? null : onClear,
                    child: const Text('Remover chave (modo texto puro)'),
                  ),
                ],
                const SizedBox(height: AppSpacing.s7),

                // ── Info callout ──────────────────────────────────────────
                const Callout(
                  icon: AppIconData.info,
                  text:
                      'A chave é armazenada localmente. '
                      'Ela deve ser idêntica à configurada no firmware do dongle (SECURE_PSK_HEX). '
                      'A chave entra em vigor na próxima conexão.',
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Badge verde/âmbar mostrando o estado atual da criptografia.
class _StatusBanner extends StatelessWidget {
  const _StatusBanner({required this.isActive});

  final bool isActive;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s5,
        vertical: AppSpacing.s4,
      ),
      decoration: BoxDecoration(
        color: isActive ? AppColors.cyan08 : AppColors.amber08,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(
          color: isActive ? AppColors.cyan14 : AppColors.amber20,
        ),
      ),
      child: Row(
        children: [
          Icon(
            isActive ? Icons.lock_rounded : Icons.lock_open_rounded,
            size: 18,
            color: isActive ? AppColors.cyan500 : AppColors.amber500,
          ),
          const SizedBox(width: AppSpacing.s3),
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                isActive ? 'Criptografia Ativa' : 'Sem Criptografia',
                style: AppTypography.ui(
                  TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: isActive ? AppColors.cyan500 : AppColors.amber500,
                  ),
                ),
              ),
              Text(
                isActive
                    ? 'AES-256-GCM · chave configurada'
                    : 'Comunicação em texto puro',
                style: AppTypography.ui(
                  const TextStyle(
                    fontSize: 11,
                    color: AppColors.textTertiary,
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
