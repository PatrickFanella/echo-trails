package api

import "testing"

func TestEvaluateBetaAccessStates(t *testing.T) {
	cities := map[string]bool{"sf": true}
	cases := []struct {
		name string
		req  InviteCodeRequest
		want BetaAccessState
	}{
		{name: "invite required", req: InviteCodeRequest{City: "sf"}, want: BetaAccessInviteRequired},
		{name: "city unavailable", req: InviteCodeRequest{City: "nyc", Code: "ABC"}, want: BetaAccessCityUnavailable},
		{name: "unknown code waitlist", req: InviteCodeRequest{City: "sf", Code: "UNKNOWN"}, want: BetaAccessWaitlist},
		{name: "accepted allowlist", req: InviteCodeRequest{City: "sf", Code: "ABC"}, want: BetaAccessAccepted},
	}
	fixtures := map[string]InviteFixture{"ABC": {Code: "ABC", State: BetaAccessAccepted}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateBetaAccess(tc.req, cities, fixtures); got.State != tc.want {
				t.Fatalf("state = %s want %s", got.State, tc.want)
			}
		})
	}
}
