import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/data/repositories/dtc_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/repositories/dtc_repository.dart';

/// Port [DtcRepository] -> impl concreta. Hoje aponta pro mock
/// ([FakeDtcRepositoryImpl]); a leitura real (datasource ELM327) troca só
/// este provider.
final Provider<DtcRepository> dtcRepositoryProvider = Provider<DtcRepository>(
  (_) => FakeDtcRepositoryImpl(),
);
