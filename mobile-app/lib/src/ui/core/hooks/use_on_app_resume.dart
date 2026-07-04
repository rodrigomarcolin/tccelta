import 'package:flutter/widgets.dart';
import 'package:flutter_hooks/flutter_hooks.dart';

/// Hook: chama [onResume] a cada retorno do app ao primeiro plano
/// (`AppLifecycleState.resumed`).
///
/// Registra um [AppLifecycleListener] UMA vez e o descarta ao desmontar; guarda
/// o callback num ref para sempre invocar a versão mais recente sem reassinar o
/// listener a cada rebuild (o `useEffect` roda só uma vez, com dependências
/// vazias).
///
/// Útil para re-checar estado que o SO só muda enquanto o app está em segundo
/// plano e que não emite stream próprio — ex.: permissões
/// (`permission_handler` é 100% pull/`Future`).
void useOnAppResume(VoidCallback onResume) {
  final callback = useRef(onResume)..value = onResume;
  useEffect(
    () {
      final listener = AppLifecycleListener(onResume: () => callback.value());
      return listener.dispose;
    },
    const [],
  );
}
