-- +goose Up
GRANT USAGE ON SCHEMA public TO tccelta_api, tccelta_worker, tccelta_provisionador, tccelta_leitura;

-- API: bloco_bruto é append-only (sem UPDATE/DELETE); demais tabelas no mínimo necessário.
GRANT SELECT                 ON dongle, pid                                    TO tccelta_api;
GRANT SELECT, UPDATE         ON dongle_estado                                  TO tccelta_api;
GRANT SELECT, INSERT         ON bloco_bruto, ancora_tempo, leitura, posicao    TO tccelta_api;
GRANT SELECT, INSERT, UPDATE ON bloco_rejeitado, desafio, boot                 TO tccelta_api;
GRANT SELECT, INSERT         ON lacuna, alerta                                 TO tccelta_api;
GRANT SELECT                 ON grupo_ancora                                   TO tccelta_api;

-- Worker: lê o contexto e grava só a reta de tempo, a fila e os alertas.
GRANT SELECT                 ON dongle, dongle_estado, bloco_bruto, ancora_tempo, lacuna, pid TO tccelta_worker;
GRANT SELECT, UPDATE         ON boot                                           TO tccelta_worker;
GRANT SELECT, UPDATE (resultado) ON desafio                                    TO tccelta_worker;
GRANT SELECT, INSERT         ON alerta                                         TO tccelta_worker;
GRANT SELECT, INSERT, DELETE ON grupo_ancora                                   TO tccelta_worker;   -- regravados a cada recálculo (spec 06)

-- O reprocessamento (tccelta worker reprocessar, spec 05) roda como tccelta_dono, dono das tabelas,
-- a partir do contêiner `ferramentas`; o worker contínuo continua sem escrever em leitura/posicao.

-- Provisionamento: cria dongles e seu estado inicial.
GRANT SELECT, INSERT         ON dongle, dongle_estado                          TO tccelta_provisionador;

-- Leitura analítica (Grafana, notebooks).
GRANT SELECT ON ALL TABLES IN SCHEMA public TO tccelta_leitura;

-- Colunas identity usam sequences.
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO tccelta_api, tccelta_worker, tccelta_provisionador;

-- +goose Down
REVOKE ALL ON ALL TABLES    IN SCHEMA public FROM tccelta_api, tccelta_worker, tccelta_provisionador, tccelta_leitura;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM tccelta_api, tccelta_worker, tccelta_provisionador;
