// Package auth handles password hashing (Argon2id) and PASETO token
// issuance/verification.
package auth

import "github.com/alexedwards/argon2id"

func HashPassword(plaintext string) (string, error) {
	return argon2id.CreateHash(plaintext, argon2id.DefaultParams)
}

func VerifyPassword(plaintext, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(plaintext, hash)
}
