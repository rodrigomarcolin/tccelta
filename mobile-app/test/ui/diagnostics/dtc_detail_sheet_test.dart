import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_freeze_frame_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_detail_sheet.dart';

void main() {
  const base = DtcCode(
    code: 'P0301',
    component: DtcComponent.engine,
    name: 'Falha de combustão — cilindro 1',
    severity: DtcSeverity.high,
    status: DtcStatus.confirmed,
  );

  Widget app(DtcCode dtc) => MaterialApp(
    theme: AppTheme.dark,
    home: Scaffold(
      body: Builder(
        builder: (context) => Center(
          child: TextButton(
            onPressed: () => showDtcDetailSheet(context, dtc: dtc),
            child: const Text('abrir'),
          ),
        ),
      ),
    ),
  );

  testWidgets(
    'sem freeze frame, a seção de congelamento fica escondida',
    (tester) async {
      await tester.pumpWidget(app(base));
      await tester.tap(find.text('abrir'));
      await tester.pumpAndSettle();

      expect(find.text('CONGELAMENTO NO MOMENTO DA FALHA'), findsNothing);
    },
  );

  testWidgets(
    'com freeze frame, a seção aparece com os valores congelados',
    (tester) async {
      const dtc = DtcCode(
        code: 'P0301',
        component: DtcComponent.engine,
        name: 'Falha de combustão — cilindro 1',
        severity: DtcSeverity.high,
        status: DtcStatus.confirmed,
        freezeFrame: [DtcFreezeFrameEntry(label: 'RPM', value: '1.500 RPM')],
      );
      await tester.pumpWidget(app(dtc));
      await tester.tap(find.text('abrir'));
      await tester.pumpAndSettle();

      expect(find.text('CONGELAMENTO NO MOMENTO DA FALHA'), findsOneWidget);
      expect(find.text('1.500 RPM'), findsOneWidget);
    },
  );
}
