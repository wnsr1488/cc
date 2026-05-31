package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	db        *pgxpool.Pool
	jwtSecret []byte
	tokenTTL  time.Duration
}

func NewService(db *pgxpool.Pool, jwtSecret string, tokenTTL time.Duration) *Service {
	return &Service{db: db, jwtSecret: []byte(jwtSecret), tokenTTL: tokenTTL}
}

func (s *Service) EnsureAdmin(ctx context.Context, username, password string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists); err != nil {
		return fmt.Errorf("check admin user: %w", err)
	}
	if exists {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, 'super_admin')
	`, username, string(hash))
	if err != nil {
		return fmt.Errorf("insert admin user: %w", err)
	}
	return nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, User, error) {
	user, err := s.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", User{}, ErrInvalidCredentials
		}
		return "", User{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", User{}, ErrInvalidCredentials
	}
	token, err := s.IssueToken(user)
	if err != nil {
		return "", User{}, err
	}
	return token, user, nil
}

func (s *Service) FindByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := s.db.QueryRow(ctx, `
		SELECT id, username, password_hash, role, created_at
		FROM users
		WHERE username = $1
	`, username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	return user, err
}

func (s *Service) FindByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := s.db.QueryRow(ctx, `
		SELECT id, username, password_hash, role, created_at
		FROM users
		WHERE id = $1
	`, id).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	return user, err
}

func (s *Service) IssueToken(user User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", user.ID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *Service) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

var ErrInvalidCredentials = errors.New("invalid username or password")
