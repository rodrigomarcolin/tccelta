import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Instância resolvida de [SharedPreferences] — sobrescrita em `main()` com a
/// instância assíncrona real antes do `runApp`. Compartilhada por qualquer
/// persistência local do app (painéis, último dongle conectado, etc.).
final Provider<SharedPreferences> sharedPreferencesProvider =
    Provider<SharedPreferences>(
      (ref) => throw UnimplementedError(
        'sharedPreferencesProvider deve ser sobrescrito em main()',
      ),
    );
