import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/showcase/view/showcase_screen.dart';

void main() {
  runApp(const TcceltaApp());
}

/// Raiz do app. Aplica o tema do OBD2 Cockpit design system e abre a galeria
/// do design system.
///
/// O wiring de `ProviderScope`/`go_router` e as features (camadas
/// `data`/`domain`/`services`) ficam para o plano de bootstrap.
class TcceltaApp extends StatelessWidget {
  /// Cria a raiz do app.
  const TcceltaApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'OBD2 Cockpit',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.dark,
      home: const ShowcaseScreen(),
    );
  }
}
