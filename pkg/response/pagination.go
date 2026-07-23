package response

type Meta struct {
	Page       int `json:"page,omitempty"`
	PerPage    int `json:"perPage,omitempty"`
	Total      int `json:"total,omitempty"`
	TotalPages int `json:"totalPages,omitempty"`
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
