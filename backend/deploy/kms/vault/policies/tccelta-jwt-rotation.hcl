path "transit-jwt/keys/jwt"        { capabilities = ["read"] }
path "transit-jwt/keys/jwt/rotate" { capabilities = ["update"] }
path "transit-jwt/keys/jwt/config" { capabilities = ["update"] }
path "transit-jwt/keys/cli"        { capabilities = ["read"] }
path "transit-jwt/keys/cli/rotate" { capabilities = ["update"] }
path "transit-jwt/keys/cli/config" { capabilities = ["update"] }
path "auth/token/lookup-self"      { capabilities = ["read"] }
path "auth/token/renew-self"       { capabilities = ["update"] }
