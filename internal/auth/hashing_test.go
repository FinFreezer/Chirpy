package auth

import (
	"log"
	"reflect"
	"testing"

	"github.com/alexedwards/argon2id"
)

func TestHash(t *testing.T) {
	var testNo int = 1
	type test struct {
		password   string
		resultHash string
		finalHash  bool
	}

	tests := []test{
		{password: "weakpassword", resultHash: "nil", finalHash: true},
		{password: "strongpassword", resultHash: "nil", finalHash: true},
		{password: "1858aijfwe!#_", resultHash: "nil", finalHash: true},
		{password: "hunter2", resultHash: "nil", finalHash: false},
	}

	for _, testCase := range tests {
		var err error = nil
		if testNo == 4 {
			testCase.resultHash, err = argon2id.CreateHash("someRandomBS", argon2id.DefaultParams)
		} else {
			testCase.resultHash, err = argon2id.CreateHash(testCase.password, argon2id.DefaultParams)
		}

		if err != nil {
			log.Printf("Error with creating hash: %s", err)
		}
		match, err := argon2id.ComparePasswordAndHash(testCase.password, testCase.resultHash)
		if err != nil {
			log.Printf("Error with comparing password and hatch: %s", err)
		}
		if !reflect.DeepEqual(testCase.finalHash, match) {
			t.Fatalf("expected %v, got: %v in test %d", testCase.finalHash, match, testNo)
		}
		testNo += 1
	}
}
