package transport

import "time"

// Unified error envelope: { "error": { "code": "...", "message": "..." } }
type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"    example:"invalid_credentials"`
	Message string `json:"message" example:"Неверный email или пароль"`
}

// Register
type registerRequest struct {
	Email       string `json:"email"       binding:"required" example:"user@example.com"`
	Password    string `json:"password"    binding:"required" example:"MyP@ssword123"`
	DisplayName string `json:"displayName" binding:"required" example:"Иван Петров"`
}

type registerResponse struct {
	Email string `json:"email" example:"user@example.com"`
}

// Login
type loginRequest struct {
	Email    string `json:"email"    binding:"required" example:"user@example.com"`
	Password string `json:"password" binding:"required" example:"MyP@ssword123"`
}

type loginResponse struct {
	ID          string `json:"id"          example:"550e8400-e29b-41d4-a716-446655440000"`
	Email       string `json:"email"       example:"user@example.com"`
	DisplayName string `json:"displayName" example:"Иван Петров"`
}

// Me (GET)
type meResponse struct {
	ID          string    `json:"id"          example:"550e8400-e29b-41d4-a716-446655440000"`
	Email       string    `json:"email"       example:"user@example.com"`
	DisplayName string    `json:"displayName" example:"Иван Петров"`
	Status      string    `json:"status"      example:"active"`
	CreatedAt   time.Time `json:"createdAt"`
}

// PATCH /api/auth/me
type updateMeRequest struct {
	DisplayName string `json:"displayName" binding:"required" example:"Новое Имя"`
}

// POST /api/auth/verification/resend
type resendRequest struct {
	Email string `json:"email" binding:"required" example:"user@example.com"`
}

// POST /api/auth/password/forgot
type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required" example:"user@example.com"`
}

// POST /api/auth/password/reset
type resetPasswordRequest struct {
	Token                string `json:"token" binding:"required" example:"q83s..."`
	Password             string `json:"password,omitempty" example:"NewValidPass123!"`
	NewPassword          string `json:"newPassword,omitempty" example:"NewValidPass123!"`
	PasswordConfirmation string `json:"passwordConfirmation,omitempty" example:"NewValidPass123!"`
}

// POST /api/auth/oauth/link — подтверждение привязки OAuth-провайдера к существующему email-аккаунту.
type oauthLinkRequest struct {
	LinkToken string `json:"link_token" binding:"required" example:"abcdef..."`
	Password  string `json:"password"   binding:"required" example:"MyP@ssword123"`
}
