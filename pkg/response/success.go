package response

import "net/http"

func OK[T any](w http.ResponseWriter, data T) {
	writeJSON(w, http.StatusOK, Response[T]{
		Success: true,
		Data:    data,
	})
}

func Created[T any](w http.ResponseWriter, data T) {
	writeJSON(w, http.StatusCreated, Response[T]{
		Success: true,
		Data:    data,
	})
}

func Accepted[T any](w http.ResponseWriter, data T) {
	writeJSON(w, http.StatusAccepted, Response[T]{
		Success: true,
		Data:    data,
	})
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func Paginated[T any](w http.ResponseWriter, data T, meta Meta) {
	writeJSON(w, http.StatusOK, Response[T]{
		Success: true,
		Data:    data,
		Meta:    &meta,
	})
}
