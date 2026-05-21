package auth

import (
	"github.com/alexedwards/argon2id"
)

func CreatePasswordHash(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}
