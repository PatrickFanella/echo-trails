package api

type BetaAccessState string

const (
	BetaAccessInviteRequired  BetaAccessState = "invite_required"
	BetaAccessCityUnavailable BetaAccessState = "city_unavailable"
	BetaAccessWaitlist        BetaAccessState = "waitlist"
	BetaAccessAccepted        BetaAccessState = "accepted"
)

type InviteCodeRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
	City  string `json:"city"`
}

type BetaAccessResponse struct {
	State   BetaAccessState `json:"state"`
	City    string          `json:"city,omitempty"`
	Message string          `json:"message"`
}
