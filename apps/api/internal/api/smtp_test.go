package api

import (
	"net/url"
	"testing"
)

func TestBuildAuthLinkEncodesPasswordlessCredentials(t *testing.T) {
	link, err := buildAuthLink("https://echotrails.example/sign-in", "request/with spaces", "12&3456")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/sign-in/auth/email/verify" || parsed.Query().Get("request_id") != "request/with spaces" || parsed.Query().Get("code") != "12&3456" {
		t.Fatalf("unexpected auth link: %s", link)
	}
}

func TestBuildAuthLinkRequiresHTTPSOutsideLocalDevelopment(t *testing.T) {
	if _, err := buildAuthLink("http://echotrails.example", "request", "123456"); err == nil {
		t.Fatal("expected insecure production link to be rejected")
	}
	if _, err := buildAuthLink("http://127.0.0.1:8080", "request", "123456"); err != nil {
		t.Fatalf("local development link rejected: %v", err)
	}
}
