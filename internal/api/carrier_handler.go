package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wyw14/cry-161/internal/model"
	"github.com/wyw14/cry-161/internal/service"
)

func listCarriers(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, svc.Carriers())
	}
}

func carrierStatus(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := svc.CarrierState(chi.URLParam(r, "carrierID"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, state)
	}
}

type carrierActionRequest struct {
	RunID    string `json:"run_id"`
	Site     string `json:"site"`
	Chamber  string `json:"chamber"`
	Layer    int    `json:"layer"`
	Result   string `json:"result"`
	Passed   bool   `json:"passed"`
	To       string `json:"to"`
	Reason   string `json:"reason"`
	Operator string `json:"operator"`
}

func (req carrierActionRequest) operatorOr(w http.Header) string {
	if req.Operator != "" {
		return req.Operator
	}
	return "local-operator"
}

func carrierEnter(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req carrierActionRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runID, ok := model.ParseID(req.RunID)
		if !ok {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		state, err := svc.CarrierEnterChamber(chi.URLParam(r, "carrierID"), runID, req.Chamber, req.operatorOr(r.Header))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, state)
	}
}

func carrierLayer(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req carrierActionRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runID, ok := model.ParseID(req.RunID)
		if !ok || req.Layer < 1 {
			http.Error(w, "invalid run id or layer", http.StatusBadRequest)
			return
		}
		carrierID := chi.URLParam(r, "carrierID")
		state, err := svc.CarrierState(carrierID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if !model.StageAtLeast(state.Stage, model.StageChamberIn) {
			http.Error(w, "carrier has not entered a chamber", http.StatusConflict)
			return
		}
		if _, err := svc.LinkLayerComplete(carrierID, runID, req.Layer); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		updated, _ := svc.CarrierState(carrierID)
		writeJSON(w, updated)
	}
}

func carrierInspect(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req carrierActionRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runID, ok := model.ParseID(req.RunID)
		if !ok {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		state, err := svc.CarrierInspect(chi.URLParam(r, "carrierID"), runID, req.Result, req.Passed, req.operatorOr(r.Header))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, state)
	}
}

func carrierHandoffFail(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req carrierActionRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runID, ok := model.ParseID(req.RunID)
		if !ok {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		handoff, err := svc.CarrierFailHandoff(chi.URLParam(r, "carrierID"), runID, req.To, req.Reason)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, handoff)
	}
}

func carrierHandoffConfirm(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req carrierActionRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runID, ok := model.ParseID(req.RunID)
		if !ok {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		state, err := svc.CarrierConfirmHandoff(chi.URLParam(r, "carrierID"), runID, req.Site, req.operatorOr(r.Header))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, state)
	}
}

func carrierConfirmSite(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req carrierActionRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runID, ok := model.ParseID(req.RunID)
		if !ok {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		state, err := svc.CarrierConfirmSite(chi.URLParam(r, "carrierID"), runID, req.Site, req.operatorOr(r.Header))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, state)
	}
}

func carrierUnload(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req carrierActionRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		runID, ok := model.ParseID(req.RunID)
		if !ok {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		state, err := svc.CarrierUnload(chi.URLParam(r, "carrierID"), runID, req.Site, req.operatorOr(r.Header))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, state)
	}
}
