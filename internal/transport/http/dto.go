package transport

import "time"

// Unified error envelope: { "error": { "code": "...", "message": "..." } }
type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Register
type registerRequest struct {
	Email       string `json:"email"       binding:"required"`
	Password    string `json:"password"    binding:"required"`
	DisplayName string `json:"displayName" binding:"required"`
}

type registerResponse struct {
	Email string `json:"email"`
}

// Login
type loginRequest struct {
	Email    string `json:"email"    binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

// Me (GET)
type meResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

// PATCH /api/auth/me
type updateMeRequest struct {
	DisplayName string `json:"displayName" binding:"required"`
}

// POST /api/auth/verification/resend
type resendRequest struct {
	Email string `json:"email" binding:"required"`
}
