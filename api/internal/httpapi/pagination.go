package httpapi

import (
	"net/http"
	"strconv"
)

// pageOffset reads how many rows of a list to skip: 0 to 1000000, 0 when omitted.
func pageOffset(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("offset")
	if raw == "" {
		return 0, true
	}
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 || offset > 1000000 {
		authError(w, 400, "invalid_request")
		return 0, false
	}
	return offset, true
}

// pageLimit reads the page size of a list: 1 to 100, 50 when omitted.
func pageLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 50, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		authError(w, 400, "invalid_request")
		return 0, false
	}
	return limit, true
}
