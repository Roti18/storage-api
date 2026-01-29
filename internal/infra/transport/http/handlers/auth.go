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
	Username string `json:"username"`
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

	// Validasi username & password
	if req.Username != h.cfg.AuthUsername || req.Password != h.cfg.AuthPassword {
		return c.Status(401).JSON(fiber.Map{
			"error": "invalid username or password",
		})
	}

	// Generate JWT token (valid 24 jam)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": req.Username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
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
