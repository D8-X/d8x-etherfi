package api

import (
	"net/http"

	"github.com/D8-X/d8x-etherfi/internal/etherfi"
	"github.com/go-chi/chi/v5"
)

// RegisterRoutes registers all API routes for D8X-Backend application
func RegisterRoutes(router chi.Router, app *etherfi.App) {

	router.Get("/contracts", func(w http.ResponseWriter, r *http.Request) {
		onHolderContracts(w, r, app)
	})

	router.Get("/get-balances", func(w http.ResponseWriter, r *http.Request) {
		onGetBalances(w, r, app)
	})

	router.Post("/balances", func(w http.ResponseWriter, r *http.Request) {
		onBalances(w, r, app)
	})
}
