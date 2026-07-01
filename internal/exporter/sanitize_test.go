package exporter

import (
	"strings"
	"testing"
)

func TestSanitizeAllowlist(t *testing.T) {
	raw := []byte(`{
		"projects": {"/home/kali/Desktop": {"hasTrustDialogAccepted": true}},
		"machineID": "device-fingerprint",
		"userID": "user-fingerprint",
		"oauthAccount": {"emailAddress": "me@example.com"},
		"primaryApiKey": "sk-ant-SHOULD-NOT-LEAK",
		"anthropicApiKey": "sk-ant-ALSO-SECRET",
		"someFutureTokenField": "tok-secret"
	}`)

	out, err := sanitize(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	// The one thing we must keep.
	if !strings.Contains(s, "/home/kali/Desktop") {
		t.Error("projects were dropped; migration would break")
	}
	// Nothing sensitive may survive, including fields we never enumerated.
	for _, secret := range []string{
		"sk-ant-SHOULD-NOT-LEAK", "sk-ant-ALSO-SECRET", "tok-secret",
		"machineID", "userID", "oauthAccount", "me@example.com",
	} {
		if strings.Contains(s, secret) {
			t.Errorf("sensitive value leaked into bundle: %q", secret)
		}
	}
}
