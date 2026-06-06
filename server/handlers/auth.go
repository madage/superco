package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/superco/server/models"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB        *sql.DB
	JWTSecret string
}

func NewAuthHandler(db *sql.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{DB: db, JWTSecret: jwtSecret}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	err := h.DB.QueryRow(
		"SELECT id, username, password FROM users WHERE username = $1",
		req.Username,
	).Scan(&user.ID, &user.Username, &user.Password)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := h.generateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Get the user's default workspace
	var wsID string
	h.DB.QueryRow(`SELECT id FROM workspaces WHERE user_id = $1 ORDER BY created_at ASC LIMIT 1`, user.ID).Scan(&wsID)

	c.JSON(http.StatusOK, models.LoginResp{
		Token:       token,
		User:        user,
		WorkspaceID: wsID,
	})
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req models.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	var user models.User
	err = h.DB.QueryRow(
		"INSERT INTO users (id, username, password) VALUES (gen_random_uuid()::text, $1, $2) RETURNING id, username, created_at",
		req.Username, string(hashed),
	).Scan(&user.ID, &user.Username, &user.CreatedAt)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}

	// Auto-create default workspace for new user
	wsID := uuid.New().String()
	h.DB.Exec(
		`INSERT INTO workspaces (id, user_id, name, description, created_at, updated_at)
		 VALUES ($1, $2, 'Default', 'Default workspace', NOW(), NOW())`,
		wsID, user.ID,
	)

	token, err := h.generateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusCreated, models.LoginResp{
		Token:       token,
		User:        user,
		WorkspaceID: wsID,
	})
}

func (h *AuthHandler) generateToken(userID, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.JWTSecret))
}
