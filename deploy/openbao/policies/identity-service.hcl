# identity-service: wrap/unwrap data keys (re-wrap = decrypt + encrypt), compute the
# phone blind index and login pseudonyms, seal/open login identifiers. It
# cannot read, export, rotate or configure any key.
path "transit/encrypt/identity-pii-kek" { capabilities = ["update"] }
path "transit/decrypt/identity-pii-kek" { capabilities = ["update"] }
path "transit/hmac/identity-pii-bidx"   { capabilities = ["update"] }
path "transit/hmac/identity-pii-bidx/*" { capabilities = ["update"] }

# Login identifiers (ADR-0013): pseudonym HMAC and the login vault key.
# No read/config/rotate/export/backup on either key (A7).
path "transit/hmac/identity-login-pseudonym"   { capabilities = ["update"] }
path "transit/hmac/identity-login-pseudonym/*" { capabilities = ["update"] }
path "transit/encrypt/identity-login-kek"      { capabilities = ["update"] }
path "transit/decrypt/identity-login-kek"      { capabilities = ["update"] }

# Periodic token housekeeping.
path "auth/token/renew-self"  { capabilities = ["update"] }
path "auth/token/lookup-self" { capabilities = ["read"] }
