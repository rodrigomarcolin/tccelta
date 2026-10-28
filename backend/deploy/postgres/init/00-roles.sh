#!/bin/sh
# Roda uma unica vez (volume vazio) como superusuario postgres. Le as senhas de /run/secrets/*.
set -eu
s() { cat "/run/secrets/$1"; }
psql -v ON_ERROR_STOP=1 -U postgres -d tccelta \
  -v owner_password="$(s pg_senha_dono)" -v api_password="$(s pg_senha_api)" \
  -v worker_password="$(s pg_senha_worker)" -v provisioner_password="$(s pg_senha_provisionador)" <<'SQL'
CREATE ROLE tccelta_owner       LOGIN PASSWORD :'owner_password';
CREATE ROLE tccelta_api         LOGIN PASSWORD :'api_password'    CONNECTION LIMIT 120;
CREATE ROLE tccelta_worker      LOGIN PASSWORD :'worker_password' CONNECTION LIMIT 20;
CREATE ROLE tccelta_provisioner LOGIN PASSWORD :'provisioner_password' CONNECTION LIMIT 5;
CREATE ROLE tccelta_reader NOLOGIN;
ALTER DATABASE tccelta OWNER TO tccelta_owner;
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;
GRANT ALL ON SCHEMA public TO tccelta_owner;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SQL
