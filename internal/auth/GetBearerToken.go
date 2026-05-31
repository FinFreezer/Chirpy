package auth

import (
	"net/http"
	"strings"
)

func GetBearerToken(headers http.Header) (string, error) {
	headerValues := headers.Values("Authorization")
	token := make(map[string]string)
	for _, value := range headerValues {
		tokens := strings.Split(value, " ")
		token[tokens[0]] = tokens[1]
	}
	return token["Bearer"], nil
}
