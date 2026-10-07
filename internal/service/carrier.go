package service

import (
	"fmt"

	"github.com/wyw14/cry-161/internal/model"
)

// Carriers lists the current custody state of every known carrier.
func (s *Service) Carriers() []model.CarrierState { return s.Link.List() }

// CarrierState returns the custody state of a single carrier.
func (s *Service) CarrierState(carrierID string) (model.CarrierState, error) {
	state, ok := s.Link.State(carrierID)
	if !ok {
		return model.CarrierState{}, fmt.Errorf("carrier %s not found", carrierID)
	}
	return state, nil
}

// CarrierEnterChamber records a bound carrier entering the named chamber.
func (s *Service) CarrierEnterChamber(carrierID string, runID model.ID, chamber, operator string) (model.CarrierState, error) {
	if chamber == "" {
		chamber = s.Chamber.Snapshot().Name
	}
	return s.Link.EnterChamber(carrierID, runID, chamber, operator)
}

// CarrierInspect records the post-coating inspection result for a carrier.
func (s *Service) CarrierInspect(carrierID string, runID model.ID, result string, passed bool, operator string) (model.CarrierState, error) {
	return s.Link.Inspect(carrierID, runID, result, passed, operator)
}

// LinkLayerComplete records a completed coating layer on a bound carrier.
func (s *Service) LinkLayerComplete(carrierID string, runID model.ID, layer int) (model.CarrierState, error) {
	return s.Link.LayerComplete(carrierID, runID, layer)
}

// CarrierFailHandoff records a failed transfer, keeping the carrier at its last
// confirmed position.
func (s *Service) CarrierFailHandoff(carrierID string, runID model.ID, to, reason string) (model.CarrierHandoff, error) {
	return s.Link.FailHandoff(carrierID, runID, to, reason)
}

// CarrierConfirmHandoff positively confirms a pending handoff arrival.
func (s *Service) CarrierConfirmHandoff(carrierID string, runID model.ID, site, operator string) (model.CarrierState, error) {
	return s.Link.ConfirmHandoff(carrierID, runID, site, operator)
}

// CarrierConfirmSite clears a position hold after on-site verification. This is
// mandatory after recovery and after a failed handoff.
func (s *Service) CarrierConfirmSite(carrierID string, runID model.ID, site, operator string) (model.CarrierState, error) {
	return s.Link.ConfirmSite(carrierID, runID, site, operator)
}

// CarrierUnload performs the unload handoff and releases the run binding.
func (s *Service) CarrierUnload(carrierID string, runID model.ID, site, operator string) (model.CarrierState, error) {
	state, err := s.Link.Unload(carrierID, runID, site, operator)
	if err != nil {
		return model.CarrierState{}, err
	}
	if value, ok := s.Runs.Get(runID); ok {
		s.Carrier.Unload(value.Batch)
	}
	return state, nil
}

// carrierForRun returns the carrier currently bound to the run, if any.
func (s *Service) carrierForRun(runID model.ID) (string, bool) {
	return s.Link.ActiveCarrier(runID)
}
