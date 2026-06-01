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

type InviteFixture struct {
	Code  string
	State BetaAccessState
}

type BetaAccessResponse struct {
	State   BetaAccessState `json:"state"`
	City    string          `json:"city,omitempty"`
	Message string          `json:"message"`
}

func EvaluateBetaAccess(req InviteCodeRequest, supportedCities map[string]bool, fixtures map[string]InviteFixture) BetaAccessResponse {
	if req.Code == "" {
		return BetaAccessResponse{State: BetaAccessInviteRequired, City: req.City, Message: "Invite code required for beta access."}
	}
	if !supportedCities[req.City] {
		return BetaAccessResponse{State: BetaAccessCityUnavailable, City: req.City, Message: "This city is not open for beta yet."}
	}
	fixture, ok := fixtures[req.Code]
	if !ok {
		return BetaAccessResponse{State: BetaAccessWaitlist, City: req.City, Message: "Invite accepted for waitlist."}
	}
	return BetaAccessResponse{State: fixture.State, City: req.City, Message: "Invite fixture evaluated."}
}
