package response

import "net/http"

func Error(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, Response[any]{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
	})
}

func ErrorWithDetails(
	w http.ResponseWriter,
	status int,
	code string,
	message string,
	details any,
) {
	writeJSON(w, status, Response[any]{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func BadRequest(w http.ResponseWriter, message string) {
	Error(w, http.StatusBadRequest, "BAD_REQUEST", message)
}

func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnauthorized, "UNAUTHORIZED", message)
}

func Forbidden(w http.ResponseWriter, message string) {
	Error(w, http.StatusForbidden, "FORBIDDEN", message)
}

func NotFound(w http.ResponseWriter, message string) {
	Error(w, http.StatusNotFound, "NOT_FOUND", message)
}

func Conflict(w http.ResponseWriter, message string) {
	Error(w, http.StatusConflict, "CONFLICT", message)
}

func Validation(w http.ResponseWriter, details any) {
	ErrorWithDetails(
		w,
		http.StatusUnprocessableEntity,
		"VALIDATION_ERROR",
		"Validation failed",
		details,
	)
}

func Internal(w http.ResponseWriter) {
	Error(
		w,
		http.StatusInternalServerError,
		"INTERNAL_SERVER_ERROR",
		"An unexpected error occurred",
	)
}
