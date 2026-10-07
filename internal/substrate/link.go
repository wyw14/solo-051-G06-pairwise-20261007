package substrate

import (
	"fmt"
	"time"

	"github.com/wyw14/cry-161/internal/journal"
	"github.com/wyw14/cry-161/internal/model"
)

// Link records the chain of custody of substrate carriers: batch load, run
// binding, chamber entry, layer completion, inspection and the unload handoff.
//
// Durability rules:
//   - A carrier is bound to at most one active run at any time, and a run is
//     bound to at most one carrier.
//   - A failed handoff never moves the carrier: its position stays at the last
//     confirmed site until a positive site confirmation clears it.
//   - After recovery the journal is not trusted as physical evidence. Active
//     carriers are marked NeedsConfirmation and every motion is blocked until
//     an operator confirms the carrier on site; in-memory state alone can never
//     skip that check.
type Link struct {
	journal *journal.Store

	states map[string]*model.CarrierState
	// activeRun maps a bound run to the carrier that holds it.
	activeRun map[model.ID]string
}

func NewLink(store *journal.Store) *Link {
	return &Link{journal: store, states: map[string]*model.CarrierState{}, activeRun: map[model.ID]string{}}
}

// Replay rebuilds custody state from the durable journal and forces every
// active carrier through fresh on-site confirmation.
func (l *Link) Replay() error {
	events, err := l.journal.Events(0)
	if err != nil {
		return fmt.Errorf("replay carrier journal: %w", err)
	}
	l.states = map[string]*model.CarrierState{}
	l.activeRun = map[model.ID]string{}
	for _, event := range events {
		if !isCarrierEvent(event.Kind) {
			continue
		}
		applyCarrierEvent(l.states, l.activeRun, event)
	}
	for _, state := range l.states {
		if state.Active {
			// The journal proves what we last knew, not where the carrier
			// physically is now. Demand a fresh site confirmation.
			state.Position.Confirmed = false
			state.NeedsConfirmation = true
		}
	}
	return nil
}

func isCarrierEvent(kind string) bool {
	switch kind {
	case model.CarrierBatchLoadedEvent,
		model.CarrierRunBoundEvent,
		model.CarrierChamberEnteredEvent,
		model.CarrierLayerCompletedEvent,
		model.CarrierInspectedEvent,
		model.CarrierUnloadedEvent,
		model.CarrierHandoffConfirmedEvent,
		model.CarrierHandoffFailedEvent,
		model.CarrierSiteConfirmedEvent:
		return true
	}
	return false
}

// State returns a copy of the carrier's custody state.
func (l *Link) State(carrierID string) (model.CarrierState, bool) {
	state, ok := l.states[carrierID]
	if !ok {
		return model.CarrierState{}, false
	}
	return copyState(state), true
}

func (l *Link) List() []model.CarrierState {
	out := make([]model.CarrierState, 0, len(l.states))
	for _, state := range l.states {
		out = append(out, copyState(state))
	}
	return out
}

// ActiveCarrier reports which carrier (if any) currently holds the run.
func (l *Link) ActiveCarrier(runID model.ID) (string, bool) {
	carrierID, ok := l.activeRun[runID]
	return carrierID, ok
}

// RequireSiteConfirmation flags an active carrier as needing fresh on-site
// confirmation. It is used when a run is recovered: the in-memory position is
// not trusted as physical evidence until an operator verifies it.
func (l *Link) RequireSiteConfirmation(carrierID string) bool {
	state, ok := l.states[carrierID]
	if !ok || !state.Active {
		return false
	}
	state.Position.Confirmed = false
	state.NeedsConfirmation = true
	return true
}

func (l *Link) LoadBatch(carrierID, batch, site, operator string) (model.CarrierState, error) {
	if carrierID == "" || batch == "" || site == "" {
		return model.CarrierState{}, fmt.Errorf("carrier, batch and site are required")
	}
	if state, ok := l.states[carrierID]; ok && state.Active {
		return model.CarrierState{}, fmt.Errorf("carrier %s already active with batch %s", carrierID, state.Batch)
	}
	event := journal.CarrierBatchLoaded(carrierID, batch, site)
	if err := l.journal.Append(event); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, event)
	result, _ := l.State(carrierID)
	return result, nil
}

func (l *Link) BindRun(carrierID string, runID model.ID, site, operator string) (model.CarrierState, error) {
	if runID == "" {
		return model.CarrierState{}, fmt.Errorf("run is required")
	}
	state, err := l.requireActive(carrierID)
	if err != nil {
		return model.CarrierState{}, err
	}
	if state.RunID != "" && state.RunID != runID {
		return model.CarrierState{}, fmt.Errorf("carrier %s already bound to run %s", carrierID, state.RunID)
	}
	if holder, ok := l.activeRun[runID]; ok && holder != carrierID {
		return model.CarrierState{}, fmt.Errorf("run %s already bound to carrier %s", runID, holder)
	}
	if err := requireMotionReady(state, "bind run"); err != nil {
		return model.CarrierState{}, err
	}
	if !model.StageAtLeast(state.Stage, model.StageBatchLoaded) {
		return model.CarrierState{}, fmt.Errorf("batch must be loaded before binding a run")
	}
	if model.StageAtLeast(state.Stage, model.StageChamberIn) {
		return model.CarrierState{}, fmt.Errorf("run already past binding stage")
	}
	event := journal.CarrierRunBound(carrierID, runID, site)
	if err := l.journal.Append(event); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, event)
	result, _ := l.State(carrierID)
	return result, nil
}

func (l *Link) EnterChamber(carrierID string, runID model.ID, chamber, operator string) (model.CarrierState, error) {
	if chamber == "" {
		return model.CarrierState{}, fmt.Errorf("chamber is required")
	}
	state, err := l.requireBound(carrierID, runID)
	if err != nil {
		return model.CarrierState{}, err
	}
	if err := requireMotionReady(state, "enter chamber"); err != nil {
		return model.CarrierState{}, err
	}
	if !model.StageAtLeast(state.Stage, model.StageRunBound) {
		return model.CarrierState{}, fmt.Errorf("run must be bound before chamber entry")
	}
	if model.StageAtLeast(state.Stage, model.StageLayerDone) {
		return model.CarrierState{}, fmt.Errorf("carrier already recorded layer completion")
	}
	event := journal.CarrierChamberEntered(carrierID, runID, chamber)
	if err := l.journal.Append(event); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, event)
	result, _ := l.State(carrierID)
	return result, nil
}

func (l *Link) LayerComplete(carrierID string, runID model.ID, layer int) (model.CarrierState, error) {
	if layer < 1 {
		return model.CarrierState{}, fmt.Errorf("layer number is invalid")
	}
	state, err := l.requireBound(carrierID, runID)
	if err != nil {
		return model.CarrierState{}, err
	}
	if err := requireMotionReady(state, "complete layer"); err != nil {
		return model.CarrierState{}, err
	}
	if !model.StageAtLeast(state.Stage, model.StageChamberIn) {
		return model.CarrierState{}, fmt.Errorf("carrier has not entered a chamber")
	}
	if model.StageAtLeast(state.Stage, model.StageInspected) {
		return model.CarrierState{}, fmt.Errorf("carrier already inspected")
	}
	if layer <= state.LayersDone {
		return model.CarrierState{}, fmt.Errorf("layer %d already recorded", layer)
	}
	event := journal.CarrierLayerCompleted(carrierID, runID, layer)
	if err := l.journal.Append(event); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, event)
	result, _ := l.State(carrierID)
	return result, nil
}

func (l *Link) Inspect(carrierID string, runID model.ID, result string, passed bool, operator string) (model.CarrierState, error) {
	state, err := l.requireBound(carrierID, runID)
	if err != nil {
		return model.CarrierState{}, err
	}
	if !model.StageAtLeast(state.Stage, model.StageLayerDone) {
		return model.CarrierState{}, fmt.Errorf("at least one layer must complete before inspection")
	}
	if model.StageAtLeast(state.Stage, model.StageInspected) {
		return model.CarrierState{}, fmt.Errorf("carrier already inspected")
	}
	event := journal.CarrierInspected(carrierID, runID, result, passed)
	if err := l.journal.Append(event); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, event)
	updated, _ := l.State(carrierID)
	return updated, nil
}

// FailHandoff records a failed transfer. The carrier stays at its last
// confirmed site and a pending handoff blocks further motion until an operator
// confirms the carrier on site.
func (l *Link) FailHandoff(carrierID string, runID model.ID, to, reason string) (model.CarrierHandoff, error) {
	state, err := l.requireBound(carrierID, runID)
	if err != nil {
		return model.CarrierHandoff{}, err
	}
	if to == "" {
		return model.CarrierHandoff{}, fmt.Errorf("handoff destination is required")
	}
	if err := requireMotionReady(state, "handoff"); err != nil {
		return model.CarrierHandoff{}, err
	}
	handoff := model.CarrierHandoff{
		ID:        model.NewID(),
		CarrierID: carrierID,
		RunID:     runID,
		From:      state.Position.Site,
		To:        to,
		Confirmed: false,
		Detail:    reason,
		At:        time.Now().UTC(),
	}
	event := journal.CarrierHandoffFailed(handoff, reason)
	if err := l.journal.Append(event); err != nil {
		return model.CarrierHandoff{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, event)
	return handoff, nil
}

// ConfirmHandoff positively confirms the carrier arrived at the handoff
// destination. Nothing assumes arrival from memory: the reported site is the
// physical confirmation and must match the pending destination.
func (l *Link) ConfirmHandoff(carrierID string, runID model.ID, site, operator string) (model.CarrierState, error) {
	state, err := l.requireBound(carrierID, runID)
	if err != nil {
		return model.CarrierState{}, err
	}
	if state.Pending == nil {
		return model.CarrierState{}, fmt.Errorf("no pending handoff for carrier %s", carrierID)
	}
	if site == "" || operator == "" {
		return model.CarrierState{}, fmt.Errorf("confirmed site and operator are required")
	}
	if site != state.Pending.To {
		return model.CarrierState{}, fmt.Errorf("carrier confirmed at %s, handoff destination was %s", site, state.Pending.To)
	}
	confirmed := *state.Pending
	confirmed.Confirmed = true
	confirmed.ConfirmedAt = time.Now().UTC()
	confirmEvent := journal.CarrierHandoffConfirmed(confirmed)
	if err := l.journal.Append(confirmEvent); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, confirmEvent)
	siteEvent := journal.CarrierSiteConfirmed(carrierID, runID, site, operator)
	if err := l.journal.Append(siteEvent); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, siteEvent)
	result, _ := l.State(carrierID)
	return result, nil
}

// ConfirmSite clears the post-recovery (or failed-handoff) hold after an
// operator verifies the carrier at its reported site. The site must equal the
// last confirmed position: stale memory can never relocate a carrier by itself.
func (l *Link) ConfirmSite(carrierID string, runID model.ID, site, operator string) (model.CarrierState, error) {
	state, err := l.requireBound(carrierID, runID)
	if err != nil {
		return model.CarrierState{}, err
	}
	if site == "" || operator == "" {
		return model.CarrierState{}, fmt.Errorf("site and operator are required")
	}
	if !state.NeedsConfirmation && state.Pending == nil && state.Position.Confirmed {
		return model.CarrierState{}, fmt.Errorf("carrier does not require site confirmation")
	}
	expected := state.Position.Site
	if state.Pending != nil {
		expected = state.Pending.From
	}
	if site != expected {
		return model.CarrierState{}, fmt.Errorf("site mismatch: operator reports %s, last confirmed site is %s", site, expected)
	}
	event := journal.CarrierSiteConfirmed(carrierID, runID, site, operator)
	if err := l.journal.Append(event); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, event)
	result, _ := l.State(carrierID)
	return result, nil
}

// Unload performs the unload handoff: the carrier must be inspected and
// positively confirmed at the unload site, after which the run binding is
// released.
func (l *Link) Unload(carrierID string, runID model.ID, site, operator string) (model.CarrierState, error) {
	state, err := l.requireBound(carrierID, runID)
	if err != nil {
		return model.CarrierState{}, err
	}
	if !model.StageAtLeast(state.Stage, model.StageInspected) {
		return model.CarrierState{}, fmt.Errorf("carrier must be inspected before unload")
	}
	if err := requireMotionReady(state, "unload"); err != nil {
		return model.CarrierState{}, err
	}
	if site != "" && state.Position.Site != "" && site != state.Position.Site {
		return model.CarrierState{}, fmt.Errorf("unload site %s does not match confirmed position %s", site, state.Position.Site)
	}
	finalSite := site
	if finalSite == "" {
		finalSite = state.Position.Site
	}
	event := journal.CarrierUnloaded(carrierID, runID, finalSite)
	if err := l.journal.Append(event); err != nil {
		return model.CarrierState{}, err
	}
	applyCarrierEvent(l.states, l.activeRun, event)
	result, _ := l.State(carrierID)
	return result, nil
}

func (l *Link) requireActive(carrierID string) (*model.CarrierState, error) {
	state, ok := l.states[carrierID]
	if !ok || !state.Active {
		return nil, fmt.Errorf("carrier %s has no active batch", carrierID)
	}
	return state, nil
}

func (l *Link) requireBound(carrierID string, runID model.ID) (*model.CarrierState, error) {
	state, err := l.requireActive(carrierID)
	if err != nil {
		return nil, err
	}
	if state.RunID == "" {
		return nil, fmt.Errorf("carrier %s is not bound to a run", carrierID)
	}
	if runID != "" && state.RunID != runID {
		return nil, fmt.Errorf("carrier %s bound to run %s, not %s", carrierID, state.RunID, runID)
	}
	return state, nil
}

func requireMotionReady(state *model.CarrierState, action string) error {
	if state.Pending != nil {
		return fmt.Errorf("cannot %s: unresolved failed handoff at %s", action, state.Position.Site)
	}
	if state.NeedsConfirmation || !state.Position.Confirmed {
		return fmt.Errorf("cannot %s: carrier position at %s requires on-site confirmation", action, state.Position.Site)
	}
	return nil
}

func copyState(state *model.CarrierState) model.CarrierState {
	copy := *state
	if state.Pending != nil {
		pending := *state.Pending
		copy.Pending = &pending
	}
	return copy
}
