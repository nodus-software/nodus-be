package response

type Meta struct {
	Page       int `json:"page"`
	PerPage    int `json:"perPage"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

func NewMeta(page, perPage, total int) Meta {
	totalPages := 0

	if perPage > 0 {
		totalPages = (total + perPage - 1) / perPage
	}

	return Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}
}
