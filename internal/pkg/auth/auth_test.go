package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
)

var (
	testPrivateKey   *rsa.PrivateKey
	testPublicKey    *rsa.PublicKey
	testPublicKeyPEM []byte
)

func TestMain(m *testing.M) {
	var err error
	testPrivateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("failed to generate test key: " + err.Error())
	}
	testPublicKey = &testPrivateKey.PublicKey

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(testPublicKey)
	if err != nil {
		panic("failed to marshal test public key: " + err.Error())
	}
	testPublicKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	os.Exit(m.Run())
}

func generateTestToken(claims *UserClaims, t *testing.T) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(testPrivateKey)
	if err != nil {
		t.Fatalf("Failed to sign test token: %v", err)
	}
	return tokenString
}

func TestEmailClaim(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		payload := `{"email": "test@example.com"}`
		encodedPayload := jwt.EncodeSegment([]byte(payload))
		jwtString := "header." + encodedPayload + ".signature"

		email, err := EmailClaim(jwtString)
		if err != nil {
			t.Fatalf("Expected no error, but got: %v", err)
		}
		if email != "test@example.com" {
			t.Errorf("Expected email 'test@example.com', but got '%s'", email)
		}
	})

	t.Run("invalid JWT segments", func(t *testing.T) {
		_, err := EmailClaim("just.one.part")
		if err == nil {
			t.Fatal("Expected an error for an invalid number of segments, but got nil")
		}
	})

	t.Run("malformed base64 payload", func(t *testing.T) {
		_, err := EmailClaim("header.%%%%.signature")
		if err == nil {
			t.Fatal("Expected an error for a malformed payload, but got nil")
		}
	})

	t.Run("payload missing email claim", func(t *testing.T) {
		payload := `{"name": "test user"}`
		encodedPayload := jwt.EncodeSegment([]byte(payload))
		jwtString := "header." + encodedPayload + ".signature"

		_, err := EmailClaim(jwtString)
		if err != ErrInvalidTokenClaims {
			t.Fatalf("Expected error '%v', but got '%v'", ErrInvalidTokenClaims, err)
		}
	})
}

func TestCheckLocalToken(t *testing.T) {
	originalKey := publicJwtKeyPEM
	publicJwtKeyPEM = testPublicKeyPEM
	t.Cleanup(func() {
		publicJwtKeyPEM = originalKey
	})

	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "test.token")

	t.Run("valid token file", func(t *testing.T) {
		claims := &UserClaims{StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()}}
		tokenString := generateTestToken(claims, t)
		os.WriteFile(tokenPath, []byte(tokenString), 0644)

		readToken, err := checkLocalToken(tokenPath)
		if err != nil {
			t.Fatalf("Expected no error for a valid token, but got: %v", err)
		}
		if readToken != tokenString {
			t.Error("Returned token does not match original token")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		claims := &UserClaims{StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(-time.Hour).Unix()}}
		tokenString := generateTestToken(claims, t)
		os.WriteFile(tokenPath, []byte(tokenString), 0644)

		_, err := checkLocalToken(tokenPath)
		if err == nil || !strings.Contains(err.Error(), "token is expired") {
			t.Fatalf("Expected an expiry error, but got: %v", err)
		}
	})

	t.Run("malformed token file", func(t *testing.T) {
		os.WriteFile(tokenPath, []byte("this is not a jwt"), 0644)
		_, err := checkLocalToken(tokenPath)
		if err == nil {
			t.Fatal("Expected an error for a malformed token, but got nil")
		}
	})

	t.Run("token signed with wrong key", func(t *testing.T) {
		otherPrivateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, &UserClaims{})
		tokenString, _ := token.SignedString(otherPrivateKey)
		os.WriteFile(tokenPath, []byte(tokenString), 0644)

		_, err := checkLocalToken(tokenPath)
		if err == nil || !strings.Contains(err.Error(), "crypto/rsa: verification error") {
			t.Fatalf("Expected a signature verification error, but got: %v", err)
		}
	})

	t.Run("token file does not exist", func(t *testing.T) {
		_, err := checkLocalToken("/path/that/does/not/exist.token")
		if err == nil {
			t.Fatal("Expected an error for a missing file, but got nil")
		}
	})
}

func TestLogin_LocalTokenExists(t *testing.T) {
	originalKey := publicJwtKeyPEM
	publicJwtKeyPEM = testPublicKeyPEM
	t.Cleanup(func() {
		publicJwtKeyPEM = originalKey
	})

	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "login.token")

	expectedToken := generateTestToken(&UserClaims{
		StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}, t)
	os.WriteFile(tokenPath, []byte(expectedToken), 0644)

	token, err := Login(context.Background(), "http://dummy-addr", tokenPath)

	if err != nil {
		t.Fatalf("Login failed when a valid local token exists: %v", err)
	}
	if token != expectedToken {
		t.Errorf("Login returned an incorrect token. Got %s, want %s", token, expectedToken)
	}
}

func TestAuthenticationFlow_EndToEnd(t *testing.T) {
	originalKey := publicJwtKeyPEM
	publicJwtKeyPEM = testPublicKeyPEM
	t.Cleanup(func() {
		publicJwtKeyPEM = originalKey
	})

	expectedClaims := &UserClaims{
		StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()},
		Email:          "final-user@example.com",
	}
	finalTokenString := generateTestToken(expectedClaims, t)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/rules":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(DeviceAuth{
				DeviceCode: "test-device-code",
				ExpiresIn:  60,
				Interval:   0,
			})
		case "/v1/auth/token_poll_rules":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TokenPollResponse{
				AccessToken: "dummy-access-token",
				IdToken:     "dummy-id-token",
				OrgUuid:     "dummy-org-uuid",
			})
		case "/v1/auth/exchange_rules":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Token{
				Token: finalTokenString,
				Type:  TokenTypePrequel,
			})
		default:
			http.NotFound(w, r)
			t.Errorf("Received unexpected request to path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(mockServer.Close)

	deviceAuth, err := startAuth(context.Background(), mockServer.URL+"/v1/auth/rules")
	if err != nil {
		t.Fatalf("startAuth failed: %v", err)
	}

	tokenPollResponse, err := pollToken(context.Background(), mockServer.URL, deviceAuth)
	if err != nil {
		t.Fatalf("pollToken failed: %v", err)
	}

	finalToken, err := exchangeRulesToken(context.Background(), mockServer.URL, tokenPollResponse)
	if err != nil {
		t.Fatalf("exchangeRulesToken failed: %v", err)
	}

	if finalToken.Token != finalTokenString {
		t.Errorf("Final token does not match expected. Got %s, want %s", finalToken.Token, finalTokenString)
	}
}
