package auth

import (
	"fmt"
	"net/http"
)

func GetBearerToken(headers http.Header) (string, error) {
	headerValues := headers.Values("Authorization")
	for _, value := range headerValues {
		fmt.Println(value)
	}
	return "", nil
}
