package auth

import _ "embed"

//go:embed public_jwt.key
var publicJwtKeyPEM []byte
