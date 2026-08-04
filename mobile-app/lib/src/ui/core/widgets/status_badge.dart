import 'dart:async';

import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';

/// Tom de um [StatusBadge] — carrega o significado do estado.
enum StatusTone {
  /// Ao vivo (feed ativo) — ciano + glow.
  live,

  /// Ok / saudável — ciano.
  ok,

  /// Aviso — âmbar.
  warning,

  /// Alerta / erro — vermelho.
  alert,
}

/// Mapeia um [StatusTone] para os tokens de cor do design system.
///
/// Fonte única do significado-para-cor: [StatusBadge] e os átomos tonalizados
/// (`IconBadge`, `IconTile`, `Callout`) consultam estes acessores em vez de
/// repetir o `switch` de tom.
extension StatusToneX on StatusTone {
  /// Cor sólida do tom (acento/ícone/texto).
  Color get color => switch (this) {
    StatusTone.live || StatusTone.ok => AppColors.cyan500,
    StatusTone.warning => AppColors.amber500,
    StatusTone.alert => AppColors.red500,
  };

  /// Preenchimento translúcido do tom (fundo de pílula/badge/tile).
  Color get wash => switch (this) {
    StatusTone.live || StatusTone.ok => AppColors.cyan10,
    StatusTone.warning => AppColors.amber08,
    StatusTone.alert => AppColors.red20,
  };

  /// Borda translúcida do tom (anel de badge/tile/callout).
  Color get border => switch (this) {
    StatusTone.live || StatusTone.ok => AppColors.cyan28,
    StatusTone.warning => AppColors.amber20,
    StatusTone.alert => AppColors.red32,
  };

  /// `true` quando o dot do tom deve brilhar (feed ao vivo / ok).
  bool get glows => this == StatusTone.live || this == StatusTone.ok;
}

/// Pílula de status declarando a saúde de uma conexão/valor.
///
/// O tom carrega o significado (ciano = ao vivo/ok, âmbar = aviso, vermelho =
/// alerta). O dot brilha em live/ok; [pulse] o faz "respirar" para sinalizar um
/// feed ativo. Espelha o componente `StatusBadge`.
class StatusBadge extends StatelessWidget {
  /// Cria uma pílula de status com [label] e [tone] dados.
  const StatusBadge({
    required this.label,
    this.tone = StatusTone.live,
    this.pulse = false,
    super.key,
  });

  /// Texto da pílula (geralmente UPPERCASE: "AO VIVO", "AVISO", "ALERTA").
  final String label;

  /// Tom semântico. @default [StatusTone.live]
  final StatusTone tone;

  /// Animação de "respiração" para um feed ativo. @default false
  final bool pulse;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s3,
        vertical: 5,
      ),
      decoration: BoxDecoration(
        color: tone.wash,
        borderRadius: AppRadii.brPill,
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          _StatusDot(tone: tone, pulse: pulse),
          const SizedBox(width: AppSpacing.s2),
          Text(
            label,
            style: AppTypography.overline.copyWith(color: tone.color),
          ),
        ],
      ),
    );
  }
}

class _StatusDot extends StatefulWidget {
  const _StatusDot({required this.tone, required this.pulse});

  final StatusTone tone;
  final bool pulse;

  @override
  State<_StatusDot> createState() => _StatusDotState();
}

class _StatusDotState extends State<_StatusDot>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: AppMotion.durPulse,
  );

  // Animação criada uma única vez (não realocada a cada build). O easeInOut
  // dá o "respiro" suave; CurveTween dispensa o dispose() que CurvedAnimation
  // exigiria.
  late final Animation<double> _opacity = _controller
      .drive(CurveTween(curve: Curves.easeInOut))
      .drive(Tween<double>(begin: 1, end: 0.3));

  bool _reduceMotion = false;

  bool get _shouldPulse => widget.pulse && !_reduceMotion;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    // Respeita "reduzir movimento" do SO (acessibilidade). Ler aqui também
    // garante a criação do controller fora do dispose() e re-sincroniza caso
    // a preferência mude em runtime.
    _reduceMotion = context.reduceMotion;
    _syncAnimation();
  }

  @override
  void didUpdateWidget(_StatusDot oldWidget) {
    super.didUpdateWidget(oldWidget);
    _syncAnimation();
  }

  void _syncAnimation() {
    if (_shouldPulse) {
      if (!_controller.isAnimating) {
        unawaited(_controller.repeat(reverse: true));
      }
    } else {
      _controller
        ..stop()
        ..value = 1;
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final dot = Container(
      width: 7,
      height: 7,
      decoration: BoxDecoration(
        color: widget.tone.color,
        shape: BoxShape.circle,
        boxShadow: widget.tone.glows ? AppGlows.cyan : null,
      ),
    );

    if (!_shouldPulse) return dot;

    // obd-pulse: opacidade 1 -> .3 -> 1, respirando em easeInOut.
    return FadeTransition(opacity: _opacity, child: dot);
  }
}

/// A faixa fixa de conexão fixada sob a status bar nas telas ao vivo.
///
/// Átomo puramente apresentacional: mostra o [device] + um [detail] secundário
/// à esquerda e um [StatusBadge] à direita. O conteúdo é dirigido por
/// parâmetros — o fio com o estado ao vivo fica na camada de feature (ex.:
/// `TelemetryStatusBand`). Espelha `StatusBand`.
class StatusBand extends StatelessWidget {
  /// Cria a faixa de conexão.
  const StatusBand({
    this.device = 'OBD2Dongle',
    this.detail = 'ELM327 v1.5',
    this.statusLabel = 'AO VIVO',
    this.statusTone = StatusTone.live,
    this.pulse = true,
    super.key,
  });

  /// Nome do dispositivo. @default "OBD2Dongle"
  final String device;

  /// Texto secundário (ex.: versão/protocolo). Se nulo/vazio, o `·` é omitido.
  /// @default "ELM327 v1.5"
  final String? detail;

  /// Rótulo do badge de status à direita. @default "AO VIVO"
  final String statusLabel;

  /// Tom semântico do badge de status. @default [StatusTone.live]
  final StatusTone statusTone;

  /// Se o dot do badge deve "respirar" (feed ao vivo). @default true
  final bool pulse;

  @override
  Widget build(BuildContext context) {
    final detail = this.detail;
    final hasDetail = detail != null && detail.isNotEmpty;
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s5,
        vertical: AppSpacing.s3,
      ),
      decoration: const BoxDecoration(
        color: AppColors.surfaceList,
        border: Border(
          bottom: BorderSide(color: AppColors.borderHairline),
        ),
      ),
      child: Row(
        children: [
          const AppIcon(AppIconData.dongle, size: 18, color: AppColors.cyan500),
          const SizedBox(width: AppSpacing.s2),
          Expanded(
            child: Text.rich(
              TextSpan(
                children: [
                  TextSpan(text: device, style: AppTypography.label),
                  if (hasDetail)
                    TextSpan(
                      text: '  ·  $detail',
                      style: AppTypography.label.copyWith(
                        color: AppColors.textTertiary,
                      ),
                    ),
                ],
              ),
              overflow: TextOverflow.ellipsis,
            ),
          ),
          const SizedBox(width: AppSpacing.s2),
          StatusBadge(label: statusLabel, tone: statusTone, pulse: pulse),
        ],
      ),
    );
  }
}
