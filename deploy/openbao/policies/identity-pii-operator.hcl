# Operator: rotate the KEK and retire old versions. No decrypt, no export.
path "transit/keys/identity-pii-kek/rotate" { capabilities = ["update"] }
path "transit/keys/identity-pii-kek"        { capabilities = ["read"] }

# Only retirement settings. exportable / allow_plaintext_backup /
# deletion_allowed are one-way or destructive and stay out of reach.
path "transit/keys/identity-pii-kek/config" {
  capabilities       = ["update"]
  allowed_parameters = {
    "min_decryption_version" = []
    "auto_rotate_period"     = []
  }
}
