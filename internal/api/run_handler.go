package api

import (
	"encoding/json"
	"github.com/wyw14/cry-161/internal/model"
	"github.com/wyw14/cry-161/internal/service"
	"net/http"
)

func runs(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { writeJSON(w, svc.RunsList()) }
}
func startRun(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			CarrierID string `json:"carrier_id"`
			Batch     string `json:"batch"`
			RecipeID  string `json:"recipe_id"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		id, ok := model.ParseID(input.RecipeID)
		if !ok {
			http.Error(w, "invalid recipe", 400)
			return
		}
		value, err := svc.Start(r.Context(), input.CarrierID, input.Batch, id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, value)
	}
}
