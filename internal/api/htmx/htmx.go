package htmx

import (
	"net/http"
	"strings"
)

func IsRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}
