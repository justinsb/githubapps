package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jws"
)

// Implement github app auth, based on https://github.com/golang/oauth2/blob/master/google/jwt.go

// parseKey converts the binary contents of a private key file
// to an *rsa.PrivateKey. It detects whether the private key is in a
// PEM container or not. If so, it extracts the private key
// from PEM container before conversion. It only supports PEM
// containers with no passphrase.
func parseKey(key []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(key)
	if block != nil {
		key = block.Bytes
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		parsedKey, err = x509.ParsePKCS1PrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("private key should be a PEM or plain PKCS1 or PKCS8; parse error: %w", err)
		}
	}
	parsed, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is invalid type, got type %T", parsedKey)
	}
	return parsed, nil
}

func NewJWTAccessTokenSource(privateKeyPath string, issuer string) (oauth2.TokenSource, error) {
	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", privateKeyPath, err)
	}
	privateKey, err := parseKey(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	// pkID := cfg.PrivateKeyID
	ts := &jwtAccessTokenSource{
		issuer: issuer,
		// email:    cfg.Email,
		// audience: audience,
		// scopes:   scopes,
		privateKey: privateKey,
		pkID:       "1",
		// pkID: cfg.PrivateKeyID,
	}
	tok, err := ts.Token()
	if err != nil {
		return nil, err
	}
	rts := oauth2.ReuseTokenSource(tok, ts)
	return rts, nil
}

type jwtAccessTokenSource struct {
	issuer string
	// scopes []string
	privateKey *rsa.PrivateKey
	pkID       string
}

func (ts *jwtAccessTokenSource) Token() (*oauth2.Token, error) {
	iat := time.Now().Add(-1 * time.Minute)
	exp := iat.Add(6 * time.Minute)
	// scope := strings.Join(ts.scopes, " ")
	cs := &jws.ClaimSet{
		Iss: ts.issuer,
		// Sub:   ts.email,
		// Aud:   ts.audience,
		// Scope: scope,
		Iat: iat.Unix(),
		Exp: exp.Unix(),
	}
	hdr := &jws.Header{
		Algorithm: "RS256",
		Typ:       "JWT",
		KeyID:     string(ts.pkID),
	}
	msg, err := jws.Encode(hdr, cs, ts.privateKey)
	if err != nil {
		return nil, fmt.Errorf("encoding JWT: %w", err)
	}
	return &oauth2.Token{AccessToken: msg, TokenType: "Bearer", Expiry: exp}, nil
}
