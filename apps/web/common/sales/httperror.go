package sales

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// MapError translates an HTTP status code and response body into one of the
// typed CRM errors declared in this package. It is HTTP-client-agnostic (no
// resty dependency) so any CRM adapter can reuse it. Callers handle the
// network-error case themselves:
//
//	if err != nil {
//	    return fmt.Errorf("%w: %v", sales.ErrServer, err)
//	}
//	if !resp.IsSuccess() {
//	    return sales.MapError(resp.StatusCode(), resp.Body())
//	}
func MapError(status int, body []byte) error {
	msg := extractErrorMessage(body)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrAuth, msg)
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case status >= 400 && status < 500:
		return fmt.Errorf("%w: %s", ErrValidation, msg)
	default:
		return fmt.Errorf("%w: %s", ErrServer, msg)
	}
}

// extractErrorMessage pulls the human-readable message out of Odoo's
// {"error": "..."} envelope; any non-JSON body is returned trimmed as a
// fallback so callers always get something useful.
func extractErrorMessage(body []byte) string {
	var er struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &er) == nil && strings.TrimSpace(er.Error) != "" {
		return er.Error
	}
	return strings.TrimSpace(string(body))
}
