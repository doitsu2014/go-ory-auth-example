# identity-service: wrap/unwrap data keys (re-wrap = decrypt + encrypt), compute the
# phone blind index. It cannot read, export, rotate or configure any key.
path "transit/encrypt/identity-pii-kek" { capabilities = ["update"] }
path "transit/decrypt/identity-pii-kek" { capabilities = ["update"] }
path "transit/hmac/identity-pii-bidx"   { capabilities = ["update"] }
path "transit/hmac/identity-pii-bidx/*" { capabilities = ["update"] }

# Periodic token housekeeping.
path "auth/token/renew-self"  { capabilities = ["update"] }
path "auth/token/lookup-self" { capabilities = ["read"] }
