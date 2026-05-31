package auth

import (
	"time"

	"log"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer:    "chirpy-access",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
		Subject:   userID.String()}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(tokenSecret))
	return tokenStr, err
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		log.Printf("An error occurred: %s", err)
		return uuid.MustParse("00000000-0000-0000-0000-000000000000"), err
	} else if claims, ok := token.Claims.(*jwt.RegisteredClaims); ok {
		uuidUser, err := uuid.Parse(claims.Subject)
		if err != nil {
			return uuid.MustParse("00000000-0000-0000-0000-000000000000"), err
		}
		return uuidUser, nil
	} else {
		log.Printf("An error occurred: %s", err)
	}
	return uuid.MustParse("00000000-0000-0000-0000-000000000000"), err
}
