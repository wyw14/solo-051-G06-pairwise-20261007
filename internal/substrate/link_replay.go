package substrate

import (
	"time"

	"github.com/wyw14/cry-161/internal/model"
)

// applyCarrierEvent folds one durable carrier event into the in-memory custody
// maps. It is used both for live updates and for journal replay, so values are
// coerced defensively (replayed data has been through JSON).
func applyCarrierEvent(states map[string]*model.CarrierState, activeRun map[model.ID]string, event model.Event) {
	carrierID := dataString(event.Data, "carrier_id")
	if carrierID == "" {
		return
	}
	state := states[carrierID]
	if state == nil {
		state = &model.CarrierState{CarrierID: carrierID}
		states[carrierID] = state
	}
	now := event.At
	if now.IsZero() {
		now = time.Now().UTC()
	}
	state.UpdatedAt = now

	switch event.Kind {
	case model.CarrierBatchLoadedEvent:
		state.Active = true
		state.Batch = dataString(event.Data, "batch")
		state.RunID = ""
		state.Stage = model.StageBatchLoaded
		state.LayersDone = 0
		state.Inspection = ""
		state.InspectionOK = false
		state.Pending = nil
		state.NeedsConfirmation = false
		state.Position = model.CarrierPosition{Site: dataString(event.Data, "site"), Confirmed: true, At: now}

	case model.CarrierRunBoundEvent:
		state.RunID = event.RunID
		state.Stage = model.StageRunBound
		state.Active = true
		if site := dataString(event.Data, "site"); site != "" {
			state.Position = model.CarrierPosition{Site: site, Confirmed: true, At: now}
		}
		activeRun[event.RunID] = carrierID

	case model.CarrierChamberEnteredEvent:
		state.Stage = model.StageChamberIn
		state.Position = model.CarrierPosition{Site: dataString(event.Data, "chamber"), Confirmed: true, At: now}
		if event.RunID != "" {
			state.RunID = event.RunID
			activeRun[event.RunID] = carrierID
		}

	case model.CarrierLayerCompletedEvent:
		state.Stage = model.StageLayerDone
		if layer := dataInt(event.Data, "layer"); layer > state.LayersDone {
			state.LayersDone = layer
		}

	case model.CarrierInspectedEvent:
		state.Stage = model.StageInspected
		state.Inspection = dataString(event.Data, "result")
		state.InspectionOK = dataBool(event.Data, "ok")

	case model.CarrierHandoffFailedEvent:
		// Position is deliberately left untouched: the carrier remains at its
		// last confirmed site. The pending handoff blocks all motion.
		handoff := model.CarrierHandoff{
			ID:        model.ID(dataString(event.Data, "handoff_id")),
			CarrierID: carrierID,
			RunID:     event.RunID,
			From:      dataString(event.Data, "from"),
			To:        dataString(event.Data, "to"),
			Confirmed: false,
			Detail:    dataString(event.Data, "reason"),
			At:        now,
		}
		if handoff.ID == "" {
			handoff.ID = model.NewID()
		}
		state.Pending = &handoff

	case model.CarrierHandoffConfirmedEvent:
		if state.Pending != nil {
			state.Pending.Confirmed = true
			state.Pending.ConfirmedAt = now
		}
		state.Pending = nil
		state.NeedsConfirmation = false
		state.Position = model.CarrierPosition{Site: dataString(event.Data, "to"), Confirmed: true, At: now}

	case model.CarrierSiteConfirmedEvent:
		site := dataString(event.Data, "site")
		if state.Pending != nil && state.Pending.From == site {
			state.Pending = nil
		}
		state.NeedsConfirmation = false
		state.Position = model.CarrierPosition{Site: site, Confirmed: true, ConfirmedBy: dataString(event.Data, "operator"), At: now}

	case model.CarrierUnloadedEvent:
		state.Stage = model.StageUnloaded
		state.Active = false
		state.Pending = nil
		state.NeedsConfirmation = false
		state.Position = model.CarrierPosition{Site: dataString(event.Data, "site"), Confirmed: true, At: now}
		if state.RunID != "" && activeRun[state.RunID] == carrierID {
			delete(activeRun, state.RunID)
		}
	}
}

func dataString(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func dataInt(data map[string]any, key string) int {
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	return 0
}

func dataBool(data map[string]any, key string) bool {
	value, ok := data[key]
	if !ok || value == nil {
		return false
	}
	typed, _ := value.(bool)
	return typed
}
