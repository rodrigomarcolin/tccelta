import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/app_card.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/shimmer.dart';

/// Tratamento interno de um [StatCard]. Selecionado pelo construtor nomeado,
/// nunca exposto à API pública.
enum _Variant { value, progress, info, gauge }

/// O pequeno tile de dashboard para uma única leitura.
///
/// Cada tratamento tem seu construtor nomeado, expondo só os campos que usa —
/// não há como passar `pct` num [StatCard.value] nem omitir o gauge num
/// [StatCard.gauge]. Todos os números são mono + tabular.
class StatCard extends StatelessWidget {
  const StatCard._(
    this._variant, {
    required this.label,
    this.value,
    this.unit,
    this.pct = 0,
    this.colorByZone = false,
    this.gauge,
    this.loading = false,
    this.cornerAccessory,
    super.key,
  });

  /// Número grande rotulado (uma leitura).
  ///
  /// Com [loading] `true`, o slot do valor vira uma barra [Shimmer] (e o rótulo
  /// também, quando vazio) — o estado "ainda não lido" do painel, distinto de
  /// um `—` de "sem dado neste ciclo".
  const StatCard.value({
    required String label,
    Object? value,
    String? unit,
    bool loading = false,
    Widget? cornerAccessory,
    Key? key,
  }) : this._(
          _Variant.value,
          label: label,
          value: value,
          unit: unit,
          loading: loading,
          cornerAccessory: cornerAccessory,
          key: key,
        );

  /// Igual a [StatCard.value] + barra de progresso (leituras percentuais).
  /// [colorByZone] recolore a barra ciano→âmbar→vermelho conforme [pct].
  /// Com [loading] `true`, valor e barra viram [Shimmer].
  const StatCard.progress({
    required String label,
    required double pct,
    Object? value,
    String? unit,
    bool colorByZone = false,
    bool loading = false,
    Widget? cornerAccessory,
    Key? key,
  }) : this._(
          _Variant.progress,
          label: label,
          value: value,
          unit: unit,
          pct: pct,
          colorByZone: colorByZone,
          loading: loading,
          cornerAccessory: cornerAccessory,
          key: key,
        );

  /// Linha "label … valor mono" para fatos estáticos (protocolo, nº de PIDs).
  const StatCard.info({
    required String label,
    Object? value,
    Key? key,
  }) : this._(
          _Variant.info,
          label: label,
          value: value,
          key: key,
        );

  /// Gauge à esquerda + valor/unidade e label à direita. O instrumento é
  /// passado em [gauge] — qualquer `GaugeVariant` (ou widget) serve.
  /// Com [loading] `true`, o valor vira uma barra [Shimmer].
  const StatCard.gauge({
    required String label,
    required Widget gauge,
    Object? value,
    String? unit,
    bool loading = false,
    Key? key,
  }) : this._(
          _Variant.gauge,
          label: label,
          value: value,
          unit: unit,
          gauge: gauge,
          loading: loading,
          key: key,
        );

  /// Rótulo (overline) da leitura — ou o label à esquerda no `info`.
  final String label;

  /// Tratamento interno selecionado pelo construtor.
  final _Variant _variant;

  /// Valor da leitura (já formatado) — ou o valor à direita no `info`.
  final Object? value;

  /// Sufixo de unidade (ex.: "%", "°").
  final String? unit;

  /// Percentual de preenchimento 0–100 para a variante `progress`. @default 0
  final double pct;

  /// Recolore a barra ciano→âmbar→vermelho conforme `pct`. @default false
  final bool colorByZone;

  /// Instrumento renderizado à esquerda na variante `gauge`.
  final Widget? gauge;

  /// Enquanto `true`, o slot do valor (e o rótulo, se vazio) vira uma barra
  /// [Shimmer] — o estado "ainda não lido". @default false
  final bool loading;

  /// Acessório opcional no canto superior direito do rótulo (ex.: uma alça
  /// de arrastar). Divide a linha com o rótulo — que quebra para uma segunda
  /// linha quando o espaço é curto — em vez de sobrepor o texto.
  final Widget? cornerAccessory;

  @override
  Widget build(BuildContext context) {
    return AppCard(
      child: switch (_variant) {
        _Variant.info => _info(),
        _Variant.gauge => _gauge(),
        _Variant.value || _Variant.progress => _valueOrProgress(),
      },
    );
  }

  Widget _info() {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Flexible(
          child: Text(
            label,
            style: AppTypography.label.copyWith(color: AppColors.textTertiary),
            overflow: TextOverflow.ellipsis,
          ),
        ),
        const SizedBox(width: AppSpacing.s3),
        Text(
          '${value ?? '—'}',
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary,
            ),
          ),
        ),
      ],
    );
  }

  Widget _valueOrProgress() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        _labelRow(),
        const SizedBox(height: AppSpacing.s2),
        _valueRow(),
        // O espaço da barra é sempre reservado para que `value` e `progress`
        // tenham a mesma altura; na variante `value` a barra fica invisível.
        const SizedBox(height: AppSpacing.s4),
        if (_variant == _Variant.progress)
          if (loading)
            const Shimmer(
              height: _ProgressBar.height,
              borderRadius: AppRadii.brPill,
            )
          else
            _ProgressBar(pct: pct, colorByZone: colorByZone)
        else
          const SizedBox(height: _ProgressBar.height),
      ],
    );
  }

  /// Rótulo overline — ou uma barra [Shimmer] quando carregando sem rótulo
  /// conhecido (fase inicial do painel, antes de descobrir os PIDs).
  Widget _label() {
    if (loading && label.isEmpty) {
      return const Shimmer(width: 54, height: 10);
    }
    return Text(label.toUpperCase(), style: AppTypography.overline);
  }

  /// [_label] + [cornerAccessory], quando informado. O rótulo fica num
  /// [Expanded] — sobra menos largura para o texto, então ele quebra para uma
  /// segunda linha em vez de ficar por baixo do acessório.
  Widget _labelRow() {
    if (cornerAccessory == null) return _label();
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(child: _label()),
        const SizedBox(width: AppSpacing.s2),
        cornerAccessory!,
      ],
    );
  }

  Widget _gauge() {
    return Row(
      children: [
        gauge!,
        const SizedBox(width: AppSpacing.s4),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              _gaugeValue(),
              const SizedBox(height: AppSpacing.s2),
              Text(
                label,
                style: AppTypography.label
                    .copyWith(color: AppColors.textTertiary),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ),
        ),
      ],
    );
  }

  /// Número + unidade para a variante `gauge`, onde o espaço horizontal é
  /// disputado com o instrumento. Um [Wrap] joga a unidade para a linha de
  /// baixo quando não cabe ao lado; o número trunca como último recurso (nunca
  /// estoura). O alinhamento por baixo aproxima a baseline do par.
  Widget _gaugeValue() {
    if (loading) return _valueShimmer(width: 64);
    final number = Text(
      '${value ?? '—'}',
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
      style: AppTypography.mono(
        const TextStyle(
          fontSize: 30,
          fontWeight: FontWeight.w700,
          height: 1,
          color: AppColors.textPrimary,
        ),
      ),
    );
    if (unit == null || unit!.isEmpty) return number;
    return Wrap(
      crossAxisAlignment: WrapCrossAlignment.end,
      spacing: 4,
      children: [
        number,
        Text(
          unit!,
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w500,
              color: AppColors.textTertiary,
            ),
          ),
        ),
      ],
    );
  }

  /// Barra [Shimmer] ocupando a mesma altura da linha do número (30px), para o
  /// card não pular quando o valor real chega.
  Widget _valueShimmer({required double width}) {
    return SizedBox(
      height: 30,
      child: Align(
        alignment: Alignment.centerLeft,
        child: Shimmer(width: width, height: 22),
      ),
    );
  }

  /// Número grande + sufixo de unidade, compartilhado por `value`/`progress`.
  ///
  /// `Wrap` (não `Row`) pelo mesmo motivo de [_gaugeValue]: um card de
  /// largura apertada (grid de 2 colunas em telas estreitas) não tem espaço
  /// garantido para o número + unidade lado a lado — o `Wrap` joga a unidade
  /// para a linha de baixo quando não cabe, e o número trunca como último
  /// recurso (nunca estoura).
  Widget _valueRow() {
    if (loading) return _valueShimmer(width: 76);
    final number = Text(
      '${value ?? '—'}',
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
      style: AppTypography.mono(
        const TextStyle(
          fontSize: 30,
          fontWeight: FontWeight.w700,
          height: 1,
          color: AppColors.textPrimary,
        ),
      ),
    );
    if (unit == null || unit!.isEmpty) return number;
    return Wrap(
      crossAxisAlignment: WrapCrossAlignment.end,
      spacing: 4,
      children: [
        number,
        Text(
          unit!,
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w500,
              color: AppColors.textTertiary,
            ),
          ),
        ),
      ],
    );
  }
}

class _ProgressBar extends StatelessWidget {
  const _ProgressBar({required this.pct, required this.colorByZone});

  /// Altura da barra, em px. Também reservada (vazia) na variante `value`.
  static const double height = 6;

  final double pct;
  final bool colorByZone;

  @override
  Widget build(BuildContext context) {
    final fraction = (pct / 100).clamp(0.0, 1.0);
    final color = colorByZone
        ? (fraction > 0.9
            ? AppColors.red500
            : fraction > 0.78
                ? AppColors.amber500
                : AppColors.cyan500)
        : AppColors.cyan500;

    return ClipRRect(
      borderRadius: AppRadii.brPill,
      child: Container(
        height: height,
        color: AppColors.track,
        child: Align(
          alignment: Alignment.centerLeft,
          child: TweenAnimationBuilder<double>(
            tween: Tween<double>(end: fraction),
            duration: context.motion(AppMotion.durValue),
            curve: AppMotion.easeValue,
            builder: (context, f, _) => FractionallySizedBox(
              widthFactor: f,
              child: Container(color: color),
            ),
          ),
        ),
      ),
    );
  }
}
