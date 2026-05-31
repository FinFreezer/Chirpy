package auth

import (
	"log"
	"reflect"
	"strings"
	"testing"

	"time"

	"github.com/google/uuid"
)

func TestJWT(t *testing.T) {
	var testNo int = 1
	var newUserUuid = uuid.New()
	type test struct {
		userUuid    uuid.UUID
		tokenSecret string
		expiration  time.Duration
		tokenString string
	}

	tests := []test{
		{userUuid: newUserUuid, tokenSecret: "isengard", expiration: time.Duration(60 * time.Second), tokenString: ""},
		{userUuid: newUserUuid, tokenSecret: "nineeleven", expiration: time.Duration(60 * time.Second), tokenString: ""},
		{userUuid: newUserUuid, tokenSecret: "chirpmaster", expiration: time.Duration(60 * time.Second), tokenString: ""},
		{userUuid: newUserUuid, tokenSecret: "hunter2", expiration: time.Duration(60 * time.Millisecond), tokenString: ""},
	}

	for _, testCase := range tests {
		var err error = nil
		if testNo == 4 {
			testCase.tokenString, err = MakeJWT(testCase.userUuid, testCase.tokenSecret, testCase.expiration)
			time.Sleep(time.Duration(100 * time.Millisecond))
		} else {
			testCase.tokenString, err = MakeJWT(testCase.userUuid, testCase.tokenSecret, testCase.expiration)
		}

		if err != nil {
			log.Printf("Error creating JWT: %s", err)
		}
		resultUuid, err := ValidateJWT(testCase.tokenString, testCase.tokenSecret)

		if err != nil && strings.Contains(err.Error(), "expired") && testNo == 4 {
			log.Printf("Expired token caught.")
			continue
		}
		if err != nil {
			log.Printf("Error with comparing password and hatch: %s", err)
		}
		if !reflect.DeepEqual(testCase.userUuid, resultUuid) {
			t.Fatalf("expected %v, got: %v in test %d", testCase.userUuid, resultUuid, testNo)
		}
		testNo += 1
	}
}
