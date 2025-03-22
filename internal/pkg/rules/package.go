package rules

import _ "embed"

//go:embed rules_ec_public.pem
var publicRulesKeyPEM []byte
