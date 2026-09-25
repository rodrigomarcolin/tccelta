import 'dart:convert';

import 'package:tccelta_mobile/src/core/errors/last_dongle_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/last_dongle_datasource.dart';
import 'package:tccelta_mobile/src/domain/ble/last_dongle.dart';
import 'package:tccelta_mobile/src/domain/repositories/last_dongle_repository.dart';

/// Impl do [LastDongleRepository] sobre um [LastDongleDatasource] local.
///
/// *Source of truth* do último dongle salvo: é o único lugar que sabe que a
/// persistência é JSON (`jsonEncode`/`jsonDecode`) e que converte erros
/// crus de parse/storage em [LastDongleStorageFailure].
class LastDongleRepositoryImpl implements LastDongleRepository {
  /// Cria o repository sobre um [LastDongleDatasource].
  const LastDongleRepositoryImpl(this._ds);

  final LastDongleDatasource _ds;

  @override
  Future<LastDongle?> load() async {
    final raw = _ds.readRaw();
    if (raw == null) return null;
    try {
      return LastDongle.fromJson(jsonDecode(raw) as Map<String, dynamic>);
    } on Object catch (e) {
      throw LastDongleStorageFailure('Dongle salvo corrompido', cause: e);
    }
  }

  @override
  Future<void> save(LastDongle device) async {
    try {
      await _ds.writeRaw(jsonEncode(device.toJson()));
    } on Object catch (e) {
      throw LastDongleStorageFailure('Falha ao salvar o dongle', cause: e);
    }
  }

  @override
  Future<void> clear() async {
    try {
      await _ds.clearRaw();
    } on Object catch (e) {
      throw LastDongleStorageFailure(
        'Falha ao apagar o dongle salvo',
        cause: e,
      );
    }
  }
}
