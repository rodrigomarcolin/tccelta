import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_guard.dart';

void main() {
  runApp(const ProviderScope(child: TcceltaApp()));
}

/// Raiz do app. Aplica o tema do OBD2 Cockpit design system e entra pelo fluxo
/// de conexão (`01 FLUXO DE CONEXÃO`) via [appRouter].
class TcceltaApp extends StatelessWidget {
  /// Cria a raiz do app.
  const TcceltaApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp.router(
      title: 'OBD2 Cockpit',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.dark,
      routerConfig: appRouter,
      // Guarda de sessão BLE de escopo global: redireciona para "Conexão
      // perdida" a partir de qualquer tela quando uma sessão ativa cai.
      builder: (context, child) => ConnectionGuard(child: child!),
    );
  }
}
