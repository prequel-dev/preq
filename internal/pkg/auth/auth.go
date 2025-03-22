package auth

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/golang-jwt/jwt"
	"github.com/prequel-dev/detection-engine/internal/pkg/ux"
	"github.com/rs/zerolog/log"
)

const (
	baseUrl          = "https://api-dev.prequel.dev:8080"
	ruleToken        = ".ruletoken"
	retries          = uint(1)
	delay            = time.Millisecond * 100
	emailNotVerified = "email not verified"
)

const (
	TokenTypePrequel = "prequel-token"
	TokenTypeId      = "oidc-id-token"
)

var (
	ErrInvalidDeviceAuth  = errors.New("invalid device auth")
	ErrFailedToGetToken   = errors.New("failed to get token")
	ErrInvalidToken       = errors.New("invalid token")
	ErrInvalidTokenClaims = errors.New("invalid token claims")
	ErrInvalidJson        = errors.New("invalid JSON")
	ErrEmailNotVerified   = errors.New("email not verified")
	ErrAuthFailure        = errors.New("auth failure")
)

type UserClaims struct {
	jwt.StandardClaims
	Name          string `json:"name" binding:"min=1,max=50"`
	Email         string `json:"email" binding:"min=1,max=50"`
	Sub           string `json:"sub" binding:"min=1,max=50"`
	Role          string `json:"role" binding:"min=1,max=50"`
	Org           string `json:"org" binding:"min=1,max=50"`
	Type          string `json:"type" binding:"min=1,max=50"`
	Domain        string `json:"domain" binding:"min=1,max=50"`
	EmailVerified bool   `json:"email_verified"`
}

type DeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationUrl         string `json:"verification_url"`
	VerificationUri         string `json:"verification_uri"`
	VerificationUriComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type TokenPollRequest struct {
	Username   string `json:"username" binding:"required,min=1,max=128"`
	DeviceCode string `json:"device_code" binding:"required,min=1,max=64"`
	OrgUuid    string `json:"org_uuid" binding:"required,min=1,max=64"`
}

type Token struct {
	Token string `json:"token"`
	Type  string `json:"type"`
}

type TokenPollResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	TokenType        string `json:"token_type"`
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	Scope            string `json:"scope"`
	IdToken          string `json:"id_token"`
	RefreshToken     string `json:"refresh_token"`
	OrgUuid          string `json:"org_uuid"`
}

func pollToken(ctx context.Context, envUrl string, deviceAuth *DeviceAuth) (*TokenPollResponse, error) {

	url := fmt.Sprintf("%s/v1/auth/token_poll_rules", envUrl)

	tokenPollRequest := &TokenPollRequest{
		Username:   "prequel-rules",
		DeviceCode: deviceAuth.DeviceCode,
	}

	now := time.Now()
	deadline := now.Add(time.Duration(deviceAuth.ExpiresIn) * time.Second)

	for {
		time.Sleep(time.Duration(deviceAuth.Interval) * time.Second)

		if now.After(deadline) {
			log.Error().Msg("Deadline exceeded")
			break
		}

		tokenPollResponse, err := tokenPoll(ctx, url, tokenPollRequest)

		if err != nil {
			continue
		}

		if tokenPollResponse.Error != "" {
			continue
		}

		if tokenPollResponse.AccessToken != "" {
			return tokenPollResponse, nil
		}
	}

	return nil, ErrFailedToGetToken
}

func startAuth(ctx context.Context, url string) (*DeviceAuth, error) {

	httpRequest, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	httpRequest.Header.Set("Accept", "application/json")

	client := &http.Client{}

	return retry.DoWithData(
		func() (*DeviceAuth, error) {

			resp, err := client.Do(httpRequest)
			if err != nil {
				log.Error().Err(err).Msg("Fail client.Do()")
				return nil, err
			}
			defer resp.Body.Close()

			rb, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Error().Err(err).Msg("Fail read body")
				return nil, err
			}

			var deviceAuth DeviceAuth
			err = json.Unmarshal(rb, &deviceAuth)
			if err != nil {
				log.Error().Err(err).Interface("response", rb).Msg("Fail unmarshal")
				return nil, err
			}

			return &deviceAuth, nil
		},
		retry.Attempts(retries),
		retry.Delay(delay),
		retry.Context(ctx),
		retry.OnRetry(func(u uint, err error) {
			log.Error().Err(err).Uint("retry", u).Msg("Retry start auth error")
		}),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
}

func tokenPoll(ctx context.Context, url string, req *TokenPollRequest) (*TokenPollResponse, error) {

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpRequest, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	httpRequest.Header.Set("Accept", "application/json")

	client := &http.Client{}

	return retry.DoWithData(
		func() (*TokenPollResponse, error) {

			resp, err := client.Do(httpRequest)
			if err != nil {
				log.Error().Err(err).Msg("Fail client.Do()")
				return nil, err
			}
			defer resp.Body.Close()

			rb, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Error().Err(err).Msg("Fail read body")
				return nil, err
			}

			var tokenPollResponse TokenPollResponse
			err = json.Unmarshal(rb, &tokenPollResponse)
			if err != nil {
				log.Error().Err(err).Interface("response", rb).Msg("Fail unmarshal")
				return nil, err
			}

			return &tokenPollResponse, nil
		},
		retry.Attempts(retries),
		retry.Delay(delay),
		retry.Context(ctx),
		retry.OnRetry(func(u uint, err error) {
			log.Error().Err(err).Uint("retry", u).Msg("Retry token poll error")
		}),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
}

func exchangeApi(ctx context.Context, url string, req *TokenExchangeRequest) (*Token, error) {

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}

	httpRequest, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	httpRequest.Header.Set("Accept", "application/json")

	client := &http.Client{}

	return retry.DoWithData(
		func() (*Token, error) {

			resp, err := client.Do(httpRequest)
			if err != nil {
				log.Error().Err(err).Msg("Fail client.Do()")
				return nil, err
			}
			defer resp.Body.Close()

			rb, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Error().Err(err).Msg("Fail read body")
				return nil, err
			}

			if resp.StatusCode != 200 {
				log.Error().Int("status", resp.StatusCode).Str("response", string(rb)).Msg("Auth exchange api response")

				var (
					failure map[string]any
					idToken string
					msg     string
					ok      bool
				)

				err = json.Unmarshal(rb, &failure)
				if err != nil {
					log.Error().Err(err).Interface("response", rb).Msg("Fail unmarshal")
					return nil, err
				}

				if msg, ok = failure["message"].(string); !ok {
					msg = "unknown error"
				}

				if idToken, ok = failure["id_token"].(string); !ok {
					idToken = ""
				}

				// Email can be not verified on first attempt. Return the id token to help
				// the user know which email address needs to be verified.
				if msg == emailNotVerified {
					return &Token{Token: idToken, Type: TokenTypeId}, nil
				}

				return nil, ErrAuthFailure
			}

			var token Token
			err = json.Unmarshal(rb, &token)
			if err != nil {
				log.Error().Err(err).Interface("response", rb).Msg("Fail unmarshal")
				return nil, err
			}

			token.Type = TokenTypePrequel

			return &token, nil
		},
		retry.Attempts(retries),
		retry.Delay(delay),
		retry.Context(ctx),
		retry.OnRetry(func(u uint, err error) {
			log.Error().Err(err).Uint("retry", u).Msg("Retry exchange api error")
		}),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
}

func exchangeRulesToken(ctx context.Context, envUrl string, tpr *TokenPollResponse) (*Token, error) {

	var (
		// This is untrusted user input that can be tampered with
		req = &TokenExchangeRequest{
			AccessToken: tpr.AccessToken,
			IdToken:     tpr.IdToken,
			OrgUuid:     tpr.OrgUuid,
		}
		url   = fmt.Sprintf("%s/v1/auth/exchange_rules", envUrl)
		token *Token
		email string
		err   error
	)

	if token, err = exchangeApi(ctx, url, req); err != nil {
		log.Error().Err(err).Msg("Fail exchangeApi")
		return nil, err
	}

	if token.Type == TokenTypeId {
		// Ignore error and print empty string for email if not found
		email, _ = EmailClaim(token.Token)
		ux.PrintEmailVerifyNotice(email)
		return nil, ErrEmailNotVerified
	}

	if len(token.Token) == 0 || token.Type != TokenTypePrequel {
		return nil, ErrInvalidToken
	}

	return token, nil
}

func EmailClaim(idToken string) (string, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid token format")
	}

	payloadSegment := parts[1]

	// Add padding if necessary
	switch len(payloadSegment) % 4 {
	case 2:
		payloadSegment += "=="
	case 3:
		payloadSegment += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payloadSegment)
	if err != nil {
		return "", fmt.Errorf("error decoding payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return "", fmt.Errorf("error unmarshaling claims: %w", err)
	}

	email, ok := claims["email"].(string)
	if !ok {
		return "", ErrInvalidTokenClaims
	}

	return email, nil
}

type TokenExchangeRequest struct {
	AccessToken string `json:"access_token" binding:"required"`
	IdToken     string `json:"id_token" binding:"required"`
	OrgUuid     string `json:"org_uuid" binding:"required"`
}

func saveToken(token, path string) error {
	if err := os.WriteFile(path, []byte(token), 0644); err != nil {
		return err
	}

	return nil
}

func checkLocalToken(path string) (string, error) {

	var (
		token          []byte
		publicKey      *rsa.PublicKey
		validatedToken *jwt.Token
		claims         *UserClaims
		ok             bool
		err            error
	)

	if token, err = os.ReadFile(path); err != nil {
		return "", err
	}

	if publicKey, err = jwt.ParseRSAPublicKeyFromPEM(publicJwtKeyPEM); err != nil {
		log.Error().Err(err).Msg("Failed to parse public key")
		return "", err
	}

	// Validate and parse the token
	validatedToken, err = jwt.ParseWithClaims(string(token), &UserClaims{}, func(token *jwt.Token) (any, error) {
		return publicKey, nil
	})

	if err != nil {
		log.Error().Err(err).Msg("Failed to parse token")
		return "", err
	}

	if claims, ok = validatedToken.Claims.(*UserClaims); !ok || !validatedToken.Valid {
		log.Error().Msg("Invalid token claims")
		return "", ErrInvalidTokenClaims
	}

	if time.Now().Unix() > claims.ExpiresAt {
		log.Error().Msg("Token expired")
		return "", ErrInvalidToken
	}

	return validatedToken.Raw, nil
}

func Login(ctx context.Context, tokenPath string) (string, error) {

	var (
		deviceAuth *DeviceAuth
		cmd        *exec.Cmd
		uri        *url.URL
		err        error
	)

	if token, err := checkLocalToken(tokenPath); err == nil {
		return token, nil
	}

	if deviceAuth, err = startAuth(ctx, fmt.Sprintf("%s/v1/auth/rules", baseUrl)); err != nil {
		log.Error().Err(err).Msg("Failed to start device auth")
		return "", err
	}

	if deviceAuth.VerificationUriComplete != "" {
		fmt.Println(deviceAuth.VerificationUriComplete)

		if uri, err = url.Parse(deviceAuth.VerificationUriComplete); err != nil {
			log.Error().Err(err).Msg("Failed to parse verification URI")
			return "", err
		}

		if uri.Scheme != "https" {
			log.Error().Msg("Invalid verification URI")
			return "", ErrInvalidDeviceAuth
		}

		switch runtime.GOOS {
		case "linux":
			cmd = exec.Command("xdg-open", uri.String())
			cmd.Start()
		case "darwin":
			cmd = exec.Command("open", uri.String())
			cmd.Start()
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", uri.String())
			cmd.Start()
		}

	} else {
		log.Error().Msg("Invalid deviceAuth")
		return "", ErrInvalidDeviceAuth
	}

	tokenPollResponse, err := pollToken(ctx, baseUrl, deviceAuth)
	if err != nil {
		log.Error().Err(err).Msg("Fail pollToken")
		return "", err
	}

	if tokenPollResponse.Error != "" {
		log.Error().Msg("Failed to get token")
		return "", ErrInvalidDeviceAuth
	}

	token, err := exchangeRulesToken(ctx, baseUrl, tokenPollResponse)
	if err != nil {
		log.Error().Err(err).Msg("Fail exchangeForApiToken")
		return "", err
	}

	if err := saveToken(token.Token, tokenPath); err != nil {
		log.Error().Err(err).Msg("Fail saveToken")
		return "", err
	}

	return token.Token, nil
}
