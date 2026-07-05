package application

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
)

type TokenService struct {
	random io.Reader
}

func NewTokenService() TokenService {
	return TokenService{random: rand.Reader}
}

func (s TokenService) NewToken() (string, string, error) {
	var raw [32]byte
	if _, err := io.ReadFull(s.random, raw[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
