package auth

import (
	"fmt"
	"time"

	"aidanwoods.dev/go-paseto"
)

const tokenTTL = 24 * time.Hour

// TokenIssuer signs and verifies PASETO v4.public tokens (stateless —
// verification needs only the public key, no server-side session store).
type TokenIssuer struct {
	secretKey paseto.V4AsymmetricSecretKey
	publicKey paseto.V4AsymmetricPublicKey
}

// NewTokenIssuer builds an issuer from a hex-encoded ed25519 secret key.
// Pass an empty string to generate a fresh key (fine for local dev; in
// production PASETO_SECRET_KEY must be set and stable across restarts,
// otherwise every restart invalidates all outstanding tokens).
func NewTokenIssuer(secretKeyHex string) (*TokenIssuer, error) {
	var sk paseto.V4AsymmetricSecretKey
	var err error
	if secretKeyHex == "" {
		sk = paseto.NewV4AsymmetricSecretKey()
	} else {
		sk, err = paseto.NewV4AsymmetricSecretKeyFromHex(secretKeyHex)
		if err != nil {
			return nil, fmt.Errorf("invalid PASETO secret key: %w", err)
		}
	}
	return &TokenIssuer{secretKey: sk, publicKey: sk.Public()}, nil
}

// Issue returns a signed token identifying userID.
func (i *TokenIssuer) Issue(userID string) string {
	token := paseto.NewToken()
	token.SetSubject(userID)
	token.SetIssuedAt(time.Now())
	token.SetExpiration(time.Now().Add(tokenTTL))
	return token.V4Sign(i.secretKey, nil)
}

// Verify checks signature and expiry, returning the embedded userID.
func (i *TokenIssuer) Verify(tokenStr string) (userID string, err error) {
	parser := paseto.NewParser()
	token, err := parser.ParseV4Public(i.publicKey, tokenStr, nil)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	return token.GetSubject()
}
