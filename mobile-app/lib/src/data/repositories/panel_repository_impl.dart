import 'dart:convert';

import 'package:tccelta_mobile/src/core/errors/panel_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/panel_datasource.dart';
import 'package:tccelta_mobile/src/domain/repositories/panel_repository.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel_collection.dart';

/// Impl do [PanelRepository] sobre um [PanelDatasource] local.
///
/// *Source of truth* dos painéis salvos: é o único lugar que sabe que a
/// persistência é JSON (`jsonEncode`/`jsonDecode`) e que converte erros
/// crus de parse/storage em [PanelStorageFailure].
class PanelRepositoryImpl implements PanelRepository {
  /// Cria o repository sobre um [PanelDatasource].
  const PanelRepositoryImpl(this._ds);

  final PanelDatasource _ds;

  @override
  Future<PanelsState?> load() async {
    final raw = _ds.readRaw();
    if (raw == null) return null;
    try {
      return PanelsState.fromJson(jsonDecode(raw) as Map<String, dynamic>);
    } on Object catch (e) {
      throw PanelStorageFailure('Painéis salvos corrompidos', cause: e);
    }
  }

  @override
  Future<void> save(PanelsState state) async {
    try {
      await _ds.writeRaw(jsonEncode(state.toJson()));
    } on Object catch (e) {
      throw PanelStorageFailure('Falha ao salvar painéis', cause: e);
    }
  }
}
