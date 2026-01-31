package handlers

import (
	"storages-api/internal/config"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

type LoginRequest struct {
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token"`
	Message string `json:"message"`
}

// POST /api/login
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Validate password only (username is ignored)
	if req.Password != h.cfg.Password {
		return c.Status(401).JSON(fiber.Map{
			"error": "invalid password",
		})
	}

	// Generate JWT token (valid for 7 days)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "admin",
		"exp":      time.Now().Add(7 * 24 * time.Hour).Unix(), // Extended to 7 days for less frequent login
		"iat":      time.Now().Unix(),
	})

	tokenString, err := token.SignedString([]byte(h.cfg.JwtSecret))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "failed to generate token",
		})
	}

	return c.JSON(LoginResponse{
		Success: true,
		Token:   tokenString,
		Message: "login successful",
	})
}
