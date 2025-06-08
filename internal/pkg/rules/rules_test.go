package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Masterminds/semver"
	"github.com/prequel-dev/preq/internal/pkg/verz"
)

func TestShouldUpdateExe(t *testing.T) {
	originalMajor := verz.Major
	originalMinor := verz.Minor
	originalBuild := verz.Build

	verz.Major = "1"
	verz.Minor = "2"
	verz.Build = "3"

	t.Cleanup(func() {
		verz.Major = originalMajor
		verz.Minor = originalMinor
		verz.Build = originalBuild
	})

	testCases := []struct {
		name         string
		response     *RuleUpdateResponse
		shouldUpdate bool
	}{
		{
			name:         "Newer version available",
			response:     &RuleUpdateResponse{LatestExeVersion: "1.3.0"},
			shouldUpdate: true,
		},
		{
			name:         "Older version available",
			response:     &RuleUpdateResponse{LatestExeVersion: "1.2.0"},
			shouldUpdate: false,
		},
		{
			name:         "Same version available",
			response:     &RuleUpdateResponse{LatestExeVersion: "1.2.3"},
			shouldUpdate: false,
		},
		{
			name:         "Newer prerelease version",
			response:     &RuleUpdateResponse{LatestExeVersion: "1.2.4-alpha.1"},
			shouldUpdate: true,
		},
		{
			name:         "Empty version in response",
			response:     &RuleUpdateResponse{LatestExeVersion: ""},
			shouldUpdate: false,
		},
		{
			name:         "Malformed version in response",
			response:     &RuleUpdateResponse{LatestExeVersion: "not-a-version"},
			shouldUpdate: false,
		},
		{
			name:         "Nil response object",
			response:     nil,
			shouldUpdate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := shouldUpdateExe(tc.response)
			if result != tc.shouldUpdate {
				t.Errorf("Expected shouldUpdate to be %v, but got %v", tc.shouldUpdate, result)
			}
		})
	}
}

func TestShouldUpdateRules(t *testing.T) {
	currentVersion := semver.MustParse("2.5.0")

	testCases := []struct {
		name         string
		response     *RuleUpdateResponse
		shouldUpdate bool
	}{
		{
			name:         "Newer version available",
			response:     &RuleUpdateResponse{LatestRuleVersion: "2.6.0"},
			shouldUpdate: true,
		},
		{
			name:         "Older version available",
			response:     &RuleUpdateResponse{LatestRuleVersion: "2.4.9"},
			shouldUpdate: false,
		},
		{
			name:         "Same version available",
			response:     &RuleUpdateResponse{LatestRuleVersion: "2.5.0"},
			shouldUpdate: false,
		},
		{
			name:         "Empty version in response",
			response:     &RuleUpdateResponse{LatestRuleVersion: ""},
			shouldUpdate: false,
		},
		{
			name:         "Malformed version in response",
			response:     &RuleUpdateResponse{LatestRuleVersion: "not-a-version"},
			shouldUpdate: false,
		},
		{
			name:         "Nil response object",
			response:     nil,
			shouldUpdate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := shouldUpdateRules(currentVersion, tc.response)
			if result != tc.shouldUpdate {
				t.Errorf("Expected shouldUpdate to be %v, but got %v", tc.shouldUpdate, result)
			}
		})
	}
}

func TestPostUrl(t *testing.T) {
	expectedResponse := map[string]string{"status": "ok"}
	expectedToken := "my-secret-token"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected method 'POST', got '%s'", r.Method)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected 'Accept: application/json' header, got '%s'", r.Header.Get("Accept"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected 'Content-Type: application/json' header, got '%s'", r.Header.Get("Content-Type"))
		}
		authHeader := fmt.Sprintf("Bearer %s", expectedToken)
		if r.Header.Get("Authorization") != authHeader {
			t.Errorf("Expected 'Authorization' header '%s', got '%s'", authHeader, r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResponse)
	}))
	t.Cleanup(mockServer.Close)

	requestBody := []byte(`{"request_key":"request_value"}`)
	responseData, err := postUrl(context.Background(), mockServer.URL, expectedToken, requestBody, 5*time.Second)
	if err != nil {
		t.Fatalf("postUrl returned an unexpected error: %v", err)
	}

	var actualResponse map[string]string
	if err := json.Unmarshal(responseData, &actualResponse); err != nil {
		t.Fatalf("Failed to unmarshal response data: %v", err)
	}
	if status, ok := actualResponse["status"]; !ok || status != "ok" {
		t.Errorf("Response body does not match expected. Got %v", actualResponse)
	}
}
