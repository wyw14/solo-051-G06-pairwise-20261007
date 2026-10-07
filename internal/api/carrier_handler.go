package api

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/wyw14/cry-161/internal/model"
	"github.com/wyw14/cry-161/internal/service"
	"github.com/wyw14/cry-161/internal/substrate"
	"net/http"
)

func carriers(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { writeJSON(w, svc.CarrierChains()) }
}

func carrierDetail(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, events, err := svc.CarrierChain(chi.URLParam(r, "carrierID"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"status": status, "events": events})
	}
}

func loadCarrier(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Batch string `json:"batch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		status, err := svc.LoadCarrier(chi.URLParam(r, "carrierID"), input.Batch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, status)
	}
}

func bindCarrier(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			RunID string `json:"run_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runID, ok := model.ParseID(input.RunID)
		if !ok {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		status, err := svc.BindCarrierRun(chi.URLParam(r, "carrierID"), runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, status)
	}
}

func enterChamber(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := svc.CarrierEnterChamber(chi.URLParam(r, "carrierID"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, status)
	}
}

func completeLayer(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Layer int `json:"layer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		status, err := svc.CarrierCompleteLayer(chi.URLParam(r, "carrierID"), input.Layer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, status)
	}
}

func inspectCarrier(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Passed   bool   `json:"passed"`
			Detail   string `json:"detail"`
			Operator string `json:"operator"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		status, err := svc.InspectCarrier(chi.URLParam(r, "carrierID"), input.Passed, input.Detail, input.Operator)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, status)
	}
}

func handoverCarrier(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Operator  string `json:"operator"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		status, err := svc.HandoverCarrier(chi.URLParam(r, "carrierID"), input.Operator, input.Confirmed)
		if err != nil {
			w.WriteHeader(http.StatusConflict)
		}
		writeJSON(w, status)
	}
}

func confirmCarrierPosition(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Position string `json:"position"`
			Operator string `json:"operator"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		status, err := svc.ConfirmCarrierPosition(chi.URLParam(r, "carrierID"), substrate.Position(input.Position), input.Operator)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, status)
	}
}
