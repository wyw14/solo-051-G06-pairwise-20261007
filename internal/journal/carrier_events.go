package journal

import "github.com/wyw14/cry-161/internal/model"

func CarrierBatchLoaded(carrierID, batch string, site string) model.Event {
	return model.NewEvent(model.CarrierBatchLoadedEvent, "", map[string]any{"carrier_id": carrierID, "batch": batch, "site": site})
}

func CarrierRunBound(carrierID string, runID model.ID, site string) model.Event {
	return model.NewEvent(model.CarrierRunBoundEvent, runID, map[string]any{"carrier_id": carrierID, "site": site})
}

func CarrierChamberEntered(carrierID string, runID model.ID, chamber string) model.Event {
	return model.NewEvent(model.CarrierChamberEnteredEvent, runID, map[string]any{"carrier_id": carrierID, "chamber": chamber})
}

func CarrierLayerCompleted(carrierID string, runID model.ID, layer int) model.Event {
	return model.NewEvent(model.CarrierLayerCompletedEvent, runID, map[string]any{"carrier_id": carrierID, "layer": layer})
}

func CarrierInspected(carrierID string, runID model.ID, result string, passed bool) model.Event {
	return model.NewEvent(model.CarrierInspectedEvent, runID, map[string]any{"carrier_id": carrierID, "result": result, "ok": passed})
}

func CarrierUnloaded(carrierID string, runID model.ID, site string) model.Event {
	return model.NewEvent(model.CarrierUnloadedEvent, runID, map[string]any{"carrier_id": carrierID, "site": site})
}

func CarrierHandoffConfirmed(h model.CarrierHandoff) model.Event {
	return model.NewEvent(model.CarrierHandoffConfirmedEvent, h.RunID, map[string]any{
		"carrier_id": h.CarrierID, "handoff_id": h.ID, "from": h.From, "to": h.To,
	})
}

func CarrierHandoffFailed(h model.CarrierHandoff, reason string) model.Event {
	return model.NewEvent(model.CarrierHandoffFailedEvent, h.RunID, map[string]any{
		"carrier_id": h.CarrierID, "handoff_id": h.ID, "from": h.From, "to": h.To, "reason": reason,
	})
}

func CarrierSiteConfirmed(carrierID string, runID model.ID, site, operator string) model.Event {
	return model.NewEvent(model.CarrierSiteConfirmedEvent, runID, map[string]any{
		"carrier_id": carrierID, "site": site, "operator": operator,
	})
}
