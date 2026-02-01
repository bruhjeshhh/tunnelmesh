package coord

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims contains the custom claims for relay authentication.
type JWTClaims struct {
	PeerName string `json:"peer_name"`
	MeshIP   string `json:"mesh_ip"`
	jwt.RegisteredClaims
}

// TokenExpiry is the default JWT token expiry duration.
const TokenExpiry = 24 * time.Hour

// GenerateToken creates a JWT token for relay authentication.
func (s *Server) GenerateToken(peerName, meshIP string) (string, error) {
	claims := JWTClaims{
		PeerName: peerName,
		MeshIP:   meshIP,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "tunnelmesh",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.AuthToken))
}

// ValidateToken validates a JWT token and returns the claims.
func (s *Server) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.AuthToken), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
