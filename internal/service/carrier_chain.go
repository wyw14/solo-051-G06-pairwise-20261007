package service

import (
	"fmt"
	"github.com/wyw14/cry-161/internal/model"
	"github.com/wyw14/cry-161/internal/substrate"
)

func (s *Service) CarrierChains() []substrate.ChainStatus { return s.Carriers.List() }

func (s *Service) CarrierChain(carrierID string) (substrate.ChainStatus, []substrate.ChainEvent, error) {
	chain, err := s.Carriers.GetOrLoad(carrierID)
	if err != nil {
		return substrate.ChainStatus{}, nil, err
	}
	return chain.Status(), chain.Events(), nil
}

func (s *Service) LoadCarrier(carrierID string, batch string) (substrate.ChainStatus, error) {
	chain, err := s.Carriers.GetOrLoad(carrierID)
	if err != nil {
		return substrate.ChainStatus{}, err
	}
	event, err := chain.Load(batch)
	if err != nil {
		return chain.Status(), err
	}
	s.appendChainEvent(event)
	return chain.Status(), nil
}

// BindCarrierRun binds a carrier to a run. The run must exist and be
// active, and neither the carrier nor the run may already be bound.
func (s *Service) BindCarrierRun(carrierID string, runID model.ID) (substrate.ChainStatus, error) {
	value, ok := s.Runs.Get(runID)
	if !ok {
		return substrate.ChainStatus{}, fmt.Errorf("run not found")
	}
	if value.State == model.Released || value.State == model.Failed {
		return substrate.ChainStatus{}, fmt.Errorf("run %s is not active (state %s)", runID, value.State)
	}
	if binding, bound := s.Carriers.ActiveBinding(runID); bound && binding.CarrierID != carrierID {
		return substrate.ChainStatus{}, fmt.Errorf("run %s is already bound to carrier %s", runID, binding.CarrierID)
	}
	chain, err := s.Carriers.GetOrLoad(carrierID)
	if err != nil {
		return substrate.ChainStatus{}, err
	}
	event, err := chain.BindRun(runID)
	if err != nil {
		return chain.Status(), err
	}
	s.appendChainEvent(event)
	return chain.Status(), nil
}

func (s *Service) CarrierEnterChamber(carrierID string) (substrate.ChainStatus, error) {
	return s.chainStep(carrierID, func(chain *substrate.Chain) (substrate.ChainEvent, error) {
		return chain.EnterChamber()
	})
}

func (s *Service) CarrierCompleteLayer(carrierID string, layer int) (substrate.ChainStatus, error) {
	return s.chainStep(carrierID, func(chain *substrate.Chain) (substrate.ChainEvent, error) {
		return chain.CompleteLayer(layer)
	})
}

func (s *Service) InspectCarrier(carrierID string, passed bool, detail string, operator string) (substrate.ChainStatus, error) {
	return s.chainStep(carrierID, func(chain *substrate.Chain) (substrate.ChainEvent, error) {
		return chain.Inspect(passed, detail, operator)
	})
}

// HandoverCarrier unloads the carrier. A failed handover keeps the last
// confirmed position and the run binding; the failure itself is recorded.
func (s *Service) HandoverCarrier(carrierID string, operator string, confirmed bool) (substrate.ChainStatus, error) {
	return s.chainStep(carrierID, func(chain *substrate.Chain) (substrate.ChainEvent, error) {
		return chain.Handover(operator, confirmed)
	})
}

// ConfirmCarrierPosition clears the post-recovery confirmation flag with
// a physically verified position. Recovered carriers cannot advance until
// this is done — memory state alone never skips on-site confirmation.
func (s *Service) ConfirmCarrierPosition(carrierID string, position substrate.Position, operator string) (substrate.ChainStatus, error) {
	return s.chainStep(carrierID, func(chain *substrate.Chain) (substrate.ChainEvent, error) {
		return chain.ConfirmPosition(position, operator)
	})
}

func (s *Service) chainStep(carrierID string, step func(*substrate.Chain) (substrate.ChainEvent, error)) (substrate.ChainStatus, error) {
	chain, err := s.Carriers.GetOrLoad(carrierID)
	if err != nil {
		return substrate.ChainStatus{}, err
	}
	event, err := step(chain)
	if event.ID != "" {
		s.appendChainEvent(event)
	}
	if err != nil {
		return chain.Status(), err
	}
	return chain.Status(), nil
}

func (s *Service) appendChainEvent(event substrate.ChainEvent) {
	_ = s.Journal.Append(model.NewEvent("carrier."+string(event.Kind), event.RunID, map[string]any{
		"carrier_id": event.CarrierID,
		"batch":      event.Batch,
		"position":   event.Position,
		"layer":      event.Layer,
		"passed":     event.Passed,
		"operator":   event.Operator,
		"detail":     event.Detail,
	}))
}
