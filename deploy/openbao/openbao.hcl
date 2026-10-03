# OpenBao server for local development (docker compose).
# Production differs: TLS listener, auto-unseal via a cloud KMS, HA storage.
storage "file" {
  path = "/openbao/file"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = true # local compose network only; never in production
}

api_addr      = "http://openbao:8200"
disable_mlock = true
ui            = false
log_level     = "info"
