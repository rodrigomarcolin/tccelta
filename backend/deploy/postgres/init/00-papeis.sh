#!/bin/sh
# Roda uma unica vez (volume vazio) como superusuario postgres. Le as senhas de /run/secrets/*.
set -eu
s() { cat "/run/secrets/$1"; }
psql -v ON_ERROR_STOP=1 -U postgres -d tccelta \
  -v senha_dono="$(s pg_senha_dono)" -v senha_api="$(s pg_senha_api)" \
  -v senha_worker="$(s pg_senha_worker)" -v senha_prov="$(s pg_senha_provisionador)" <<'SQL'
CREATE ROLE tccelta_dono    LOGIN PASSWORD :'senha_dono';
CREATE ROLE tccelta_api     LOGIN PASSWORD :'senha_api'    CONNECTION LIMIT 120;
CREATE ROLE tccelta_worker  LOGIN PASSWORD :'senha_worker' CONNECTION LIMIT 20;
CREATE ROLE tccelta_provisionador LOGIN PASSWORD :'senha_prov' CONNECTION LIMIT 5;
CREATE ROLE tccelta_leitura NOLOGIN;
ALTER DATABASE tccelta OWNER TO tccelta_dono;
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;
GRANT ALL ON SCHEMA public TO tccelta_dono;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SQL
