#!/bin/sh
set -eu

mkdir -p /var/lib/postgresql/tls
cp /tls-source/server.key /var/lib/postgresql/tls/server.key
cp /tls-source/server.crt /var/lib/postgresql/tls/server.crt
cp /tls-source/ca.crt /var/lib/postgresql/tls/ca.crt
chown -R postgres:postgres /var/lib/postgresql/tls
chmod 0700 /var/lib/postgresql/tls
chmod 0600 /var/lib/postgresql/tls/server.key
chmod 0644 /var/lib/postgresql/tls/server.crt /var/lib/postgresql/tls/ca.crt

exec docker-entrypoint.sh postgres \
  -c ssl=on \
  -c ssl_cert_file=/var/lib/postgresql/tls/server.crt \
  -c ssl_key_file=/var/lib/postgresql/tls/server.key \
  -c ssl_ca_file=/var/lib/postgresql/tls/ca.crt \
  -c hba_file=/tls-config/pg_hba.conf \
  -c password_encryption=scram-sha-256
