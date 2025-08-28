package main

import (
	"crypto/sha256"
	"fmt"
	"slices"

	"golang.org/x/crypto/curve25519"
)

func FirstStep(key [32]byte, input []string) []MaskedEmail {
	seedPoint, err := curve25519.X25519(key[:], curve25519.Basepoint)
	if err != nil {
		panic(fmt.Sprintf("PROGRAMMING ERROR: Invalid x25519 point: %v", err))
	}
	var result []MaskedEmail
	for _, item := range input {
		emailScalar := sha256.Sum256([]byte(item))
		emailPoint, err := curve25519.X25519(emailScalar[:], seedPoint)
		if err != nil {
			panic(fmt.Sprintf("PROGRAMMING ERROR: Invalid x25519 point: %v", err))
		}
		me := MaskedEmail{}
		copy(me[:], emailPoint)
		result = append(result, me)
	}
	slices.SortStableFunc(result, MaskedEmail.Compare)
	return result
}

func SecondStep(key [32]byte, input []MaskedEmail) []MaskedEmail {
	var result []MaskedEmail
	for i := range input {
		emailPoint, err := curve25519.X25519(key[:], input[i][:])
		if err != nil {
			panic(fmt.Sprintf("PROGRAMMING ERROR: Invalid x25519 point: %v", err))
		}
		me := MaskedEmail{}
		copy(me[:], emailPoint)
		result = append(result, me)
	}
	slices.SortStableFunc(result, MaskedEmail.Compare)
	return result
}
