package users

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"

	"nodus-health/pkg/response"
)

var validate = validator.New()

// bindJSON decodes the request body into T and runs struct-tag validation.
// On failure it writes the appropriate error response itself and returns
// ok=false so the caller can return immediately.
func bindJSON[T any](w http.ResponseWriter, r *http.Request) (payload T, ok bool) {
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		response.BadRequest(w, "invalid request body")
		return payload, false
	}
	if err := validate.Struct(payload); err != nil {
		verrs, ok := err.(validator.ValidationErrors)
		if !ok {
			response.BadRequest(w, "invalid request body")
			return payload, false
		}
		response.Validation(w, formatValidationErrors(verrs))
		return payload, false
	}
	return payload, true
}

func formatValidationErrors(verrs validator.ValidationErrors) map[string]string {
	details := make(map[string]string, len(verrs))
	for _, fe := range verrs {
		field := strings.ToLower(fe.Field())
		switch fe.Tag() {
		case "required":
			details[field] = fmt.Sprintf("%s is required", field)
		case "oneof":
			details[field] = fmt.Sprintf("%s must be one of: %s", field, fe.Param())
		default:
			details[field] = fmt.Sprintf("%s is invalid", field)
		}
	}
	return details
}
