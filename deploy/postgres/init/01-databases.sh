#!/bin/sh
# Creates one database + login role per component (ADR-0007).
# Passwords come from the container environment and are passed as psql
# variables (:'var' quotes them safely); never hard-code them here.
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
  -v kratos_pw="$KRATOS_DB_PASSWORD" \
  -v keto_pw="$KETO_DB_PASSWORD" \
  -v migrator_pw="$IDENTITY_MIGRATOR_DB_PASSWORD" \
  -v app_pw="$IDENTITY_APP_DB_PASSWORD" <<'SQL'
CREATE ROLE kratos LOGIN PASSWORD :'kratos_pw';
CREATE DATABASE kratos OWNER kratos;

CREATE ROLE keto LOGIN PASSWORD :'keto_pw';
CREATE DATABASE keto OWNER keto;

CREATE ROLE identity_migrator LOGIN PASSWORD :'migrator_pw';
CREATE ROLE identity_app LOGIN PASSWORD :'app_pw';
CREATE DATABASE identity OWNER identity_migrator;

REVOKE ALL ON DATABASE kratos FROM PUBLIC;
REVOKE ALL ON DATABASE keto FROM PUBLIC;
REVOKE ALL ON DATABASE identity FROM PUBLIC;
GRANT CONNECT ON DATABASE identity TO identity_app;
SQL

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname identity <<'SQL'
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
ALTER SCHEMA public OWNER TO identity_migrator;
GRANT USAGE ON SCHEMA public TO identity_app;
SQL
