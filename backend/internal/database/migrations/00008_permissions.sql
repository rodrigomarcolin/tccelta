-- +goose Up
GRANT USAGE ON SCHEMA public TO tccelta_api, tccelta_worker, tccelta_provisioner, tccelta_reader;

-- API: raw_block é append-only (sem UPDATE/DELETE); demais tabelas no mínimo necessário.
GRANT SELECT                 ON dongle, pid                                    TO tccelta_api;
GRANT SELECT, UPDATE         ON dongle_state                                  TO tccelta_api;
GRANT SELECT, INSERT         ON raw_block, time_anchor, reading, position    TO tccelta_api;
GRANT SELECT, INSERT, UPDATE ON rejected_block, challenge, boot                 TO tccelta_api;
GRANT SELECT, INSERT         ON gap, alert                                 TO tccelta_api;
GRANT SELECT                 ON anchor_group                                   TO tccelta_api;

-- Worker: lê o contexto e grava só a reta de tempo, a fila e os alertas.
GRANT SELECT                 ON dongle, dongle_state, raw_block, time_anchor, gap, pid TO tccelta_worker;
GRANT SELECT, UPDATE         ON boot                                           TO tccelta_worker;
GRANT SELECT, UPDATE (result) ON challenge                                    TO tccelta_worker;
GRANT SELECT, INSERT         ON alert                                         TO tccelta_worker;
GRANT SELECT, INSERT, DELETE ON anchor_group                                   TO tccelta_worker;   -- regravados a cada recálculo (spec 06)

-- O reprocessamento (tccelta worker reprocess, spec 05) roda como tccelta_owner, dono das tabelas,
-- a partir do contêiner `ferramentas`; o worker contínuo continua sem escrever em leitura/posicao.

-- Provisionamento: cria dongles e seu estado inicial.
GRANT SELECT, INSERT         ON dongle, dongle_state                          TO tccelta_provisioner;

-- Leitura analítica (Grafana, notebooks).
GRANT SELECT ON ALL TABLES IN SCHEMA public TO tccelta_reader;

-- Colunas identity usam sequences.
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO tccelta_api, tccelta_worker, tccelta_provisioner;

-- +goose Down
REVOKE ALL ON ALL TABLES    IN SCHEMA public FROM tccelta_api, tccelta_worker, tccelta_provisioner, tccelta_reader;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM tccelta_api, tccelta_worker, tccelta_provisioner;
