package clinical

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"nodus-health/pkg/response"
)

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateOrderRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(actor string) error {
		x, e := h.service.CreateOrder(r.Context(), actor, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) CreateServiceOrder(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateOrderRequest](w, r)
	if !ok {
		return
	}
	q.Kind = "service"
	h.withActor(w, r, func(actor string) error {
		x, e := h.service.CreateOrder(r.Context(), actor, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) CreatePrescription(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[CreateOrderRequest](w, r)
	if !ok {
		return
	}
	q.Kind = "medication"
	h.withActor(w, r, func(actor string) error {
		x, e := h.service.CreateOrder(r.Context(), actor, chi.URLParam(r, "visitId"), q)
		if e == nil {
			response.Created(w, x)
		}
		return e
	})
}
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	x, e := h.service.ListOrders(r.Context(), OrderFilters{VisitID: strings.TrimSpace(r.URL.Query().Get("visit_id")), Category: strings.TrimSpace(r.URL.Query().Get("category")), Status: strings.TrimSpace(r.URL.Query().Get("status")), Kind: strings.TrimSpace(r.URL.Query().Get("kind")), Page: page, PerPage: per})
	if e != nil {
		h.fail(w, e)
		return
	}
	response.Paginated(w, x.Data, response.NewMeta(x.Page, x.PerPage, x.Total))
}
func (h *Handler) ListVisitOrders(w http.ResponseWriter, r *http.Request) {
	x, e := h.service.ListOrders(r.Context(), OrderFilters{VisitID: chi.URLParam(r, "visitId"), Page: 1, PerPage: 100})
	if e != nil {
		h.fail(w, e)
		return
	}
	response.OK(w, x.Data)
}
func (h *Handler) TransitionOrder(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[TransitionOrderRequest](w, r)
	if !ok {
		return
	}
	h.withActor(w, r, func(actor string) error {
		x, e := h.service.TransitionOrder(r.Context(), actor, chi.URLParam(r, "orderId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	q, ok := bind[TransitionOrderRequest](w, r)
	if !ok {
		return
	}
	q.Status = "cancelled"
	h.withActor(w, r, func(actor string) error {
		x, e := h.service.TransitionOrder(r.Context(), actor, chi.URLParam(r, "orderId"), q)
		if e == nil {
			response.OK(w, x)
		}
		return e
	})
}
