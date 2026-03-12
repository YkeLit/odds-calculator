package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"odds-calculator/backend/internal/models"
	"odds-calculator/backend/internal/storage"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserExists         = errors.New("username already exists")
	ErrInvalidToken       = errors.New("invalid token")
)

type Service struct {
	store     *storage.Store
	jwtSecret []byte
	tokenTTL  time.Duration
}

func NewService(store *storage.Store, jwtSecret string, tokenTTL time.Duration) *Service {
	return &Service{
		store:     store,
		jwtSecret: []byte(jwtSecret),
		tokenTTL:  tokenTTL,
	}
}

func (s *Service) Register(req models.RegisterRequest) (models.AuthResponse, error) {
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 32 {
		return models.AuthResponse{}, fmt.Errorf("username length must be between 3 and 32")
	}
	if len(req.Password) < 6 {
		return models.AuthResponse{}, fmt.Errorf("password length must be at least 6")
	}
	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		return models.AuthResponse{}, fmt.Errorf("hash password: %w", err)
	}
	userID, err := s.store.CreateUser(username, passwordHash)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return models.AuthResponse{}, ErrUserExists
		}
		return models.AuthResponse{}, fmt.Errorf("create user: %w", err)
	}
	return s.issueToken(userID, username)
}

func (s *Service) Login(req models.LoginRequest) (models.AuthResponse, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		return models.AuthResponse{}, ErrInvalidCredentials
	}
	user, err := s.store.GetUserByUsername(username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return models.AuthResponse{}, ErrInvalidCredentials
		}
		return models.AuthResponse{}, fmt.Errorf("find user: %w", err)
	}
	if !verifyPassword(req.Password, user.PasswordHash) {
		return models.AuthResponse{}, ErrInvalidCredentials
	}
	return s.issueToken(user.ID, user.Username)
}

func (s *Service) ParseToken(token string) (models.UserInfo, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return models.UserInfo{}, ErrInvalidToken
	}
	payloadSig := parts[0] + "." + parts[1]
	expected := signHMAC(payloadSig, s.jwtSecret)
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return models.UserInfo{}, ErrInvalidToken
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return models.UserInfo{}, ErrInvalidToken
	}
	var claims jwtClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return models.UserInfo{}, ErrInvalidToken
	}
	if claims.Exp < time.Now().Unix() {
		return models.UserInfo{}, ErrInvalidToken
	}
	if claims.Sub <= 0 || claims.Username == "" {
		return models.UserInfo{}, ErrInvalidToken
	}
	return models.UserInfo{ID: claims.Sub, Username: claims.Username}, nil
}

func (s *Service) issueToken(userID int64, username string) (models.AuthResponse, error) {
	token, err := s.generateJWT(jwtClaims{
		Sub:      userID,
		Username: username,
		Exp:      time.Now().Add(s.tokenTTL).Unix(),
	})
	if err != nil {
		return models.AuthResponse{}, fmt.Errorf("generate token: %w", err)
	}
	return models.AuthResponse{
		AccessToken: token,
		User: models.UserInfo{
			ID:       userID,
			Username: username,
		},
	}, nil
}

type jwtClaims struct {
	Sub      int64  `json:"sub"`
	Username string `json:"username"`
	Exp      int64  `json:"exp"`
}

func (s *Service) generateJWT(claims jwtClaims) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerBytes)
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadBytes)
	payloadSig := headerEncoded + "." + payloadEncoded
	sig := signHMAC(payloadSig, s.jwtSecret)
	return payloadSig + "." + sig, nil
}

func signHMAC(payload string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h := sha256.Sum256(append(salt, []byte(password)...))
	return base64.RawURLEncoding.EncodeToString(salt) + "." + base64.RawURLEncoding.EncodeToString(h[:]), nil
}

func verifyPassword(password, packed string) bool {
	parts := strings.Split(packed, ".")
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	h := sha256.Sum256(append(salt, []byte(password)...))
	return hmac.Equal(h[:], want)
}
