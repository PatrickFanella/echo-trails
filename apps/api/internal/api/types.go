package api

import "time"

type StartEmailRequest struct {
	Email string `json:"email"`
}

type StartEmailResponse struct {
	RequestID string    `json:"request_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type VerifyEmailRequest struct {
	RequestID  string `json:"request_id"`
	Code       string `json:"code"`
	DeviceName string `json:"device_name,omitempty"`
}

type VerifyEmailResponse struct {
	UserID                string    `json:"user_id"`
	SessionID             string    `json:"session_id"`
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}
type RefreshResponse = VerifyEmailResponse
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}
