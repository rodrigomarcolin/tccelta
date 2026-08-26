import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/data/repositories/obd2_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';

/// Repository = *source of truth* da telemetria OBD-II. Consome o
/// `dongleRepositoryProvider` para obter a conexão BLE viva. Não é autoDispose:
/// o `Elm327Client` interno assina a TX da conexão e deve viver com ela.
final Provider<Obd2Repository> obd2RepositoryProvider =
    Provider<Obd2Repository>((ref) {
      final repo = Obd2RepositoryImpl(ref.read(dongleRepositoryProvider));
      ref.onDispose(repo.dispose);
      return repo;
    });
