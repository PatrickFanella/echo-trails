package api

import "testing"

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "lowercases and trims", input: "  Test.User@Example.COM  ", want: "test.user@example.com"},
		{name: "display name", input: "Echo Tester <Tester@Example.com>", want: "tester@example.com"},
		{name: "invalid", input: "not-an-email", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeEmail(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("normalizeEmail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifyCodeUsesHash(t *testing.T) {
	hash := codeHash("123456")
	if hash == "123456" {
		t.Fatal("code hash stored plaintext code")
	}
	if !verifyCode(hash, "123456") {
		t.Fatal("expected matching code to verify")
	}
	if verifyCode(hash, "654321") {
		t.Fatal("expected non-matching code to fail")
	}
}

func TestRandomTokenPrefixes(t *testing.T) {
	id, err := newID("req_")
	if err != nil {
		t.Fatal(err)
	}
	if len(id) <= len("req_") || id[:4] != "req_" {
		t.Fatalf("unexpected id %q", id)
	}

	tok, err := newToken("rfr_")
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) <= len("rfr_") || tok[:4] != "rfr_" {
		t.Fatalf("unexpected token %q", tok)
	}
}
