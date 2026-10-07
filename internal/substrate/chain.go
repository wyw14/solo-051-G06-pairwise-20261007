package substrate

import (
	"fmt"
	"github.com/wyw14/cry-161/internal/model"
	"sync"
	"time"
)

// ChainStage identifies the lifecycle step a carrier has reached in the chain.
type ChainStage string

const (
	StageIdle           ChainStage = "idle"
	StageLoaded         ChainStage = "loaded"
	StageRunBound       ChainStage = "run_bound"
	StageChamberEntered ChainStage = "chamber_entered"
	StageLayerCompleted ChainStage = "layer_completed"
	StageInspected      ChainStage = "inspected"
	StageHandedOver     ChainStage = "handed_over"
)

// Position is a physical location of the carrier on the line.
type Position string

const (
	PositionUnknown  Position = "unknown"
	PositionLoadPort Position = "load_port"
	PositionChamber  Position = "chamber"
	PositionUnload   Position = "unload_port"
)

// ChainKind classifies recorded carrier chain events.
type ChainKind string

const (
	EventLoaded            ChainKind = "loaded"
	EventRunBound          ChainKind = "run_bound"
	EventChamberEntered    ChainKind = "chamber_entered"
	EventLayerCompleted    ChainKind = "layer_completed"
	EventInspected         ChainKind = "inspected"
	EventHandover          ChainKind = "handover"
	EventHandoverFailed    ChainKind = "handover_failed"
	EventPositionConfirmed ChainKind = "position_confirmed"
)

// ChainEvent is one persisted step in the carrier chain.
type ChainEvent struct {
	ID        model.ID  `json:"id"`
	CarrierID string    `json:"carrier_id"`
	Kind      ChainKind `json:"kind"`
	Batch     string    `json:"batch,omitempty"`
	RunID     model.ID  `json:"run_id,omitempty"`
	Position  Position  `json:"position,omitempty"`
	Layer     int       `json:"layer,omitempty"`
	Passed    bool      `json:"passed,omitempty"`
	Operator  string    `json:"operator,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	At        time.Time `json:"at"`
}

// ChainStatus is a point-in-time view of a carrier chain.
type ChainStatus struct {
	CarrierID         string     `json:"carrier_id"`
	Batch             string     `json:"batch,omitempty"`
	RunID             model.ID   `json:"run_id,omitempty"`
	RunActive         bool       `json:"run_active"`
	Stage             ChainStage `json:"stage"`
	Position          Position   `json:"position"`
	LayersCompleted   int        `json:"layers_completed"`
	Inspected         bool       `json:"inspected"`
	NeedsConfirmation bool       `json:"needs_confirmation"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Chain tracks one carrier through load → bind → chamber → layers →
// inspection → handover. Every step is persisted before it is applied,
// so a recovered chain never trusts memory alone: a mid-cycle carrier
// requires on-site position confirmation before it can advance again.
type Chain struct {
	mu           sync.Mutex
	carrierID    string
	batch        string
	runID        model.ID
	runActive    bool
	stage        ChainStage
	position     Position
	layers       int
	inspected    bool
	needsConfirm bool
	events       []ChainEvent
	store        *ChainStore
}

func NewChain(carrierID string, store *ChainStore) *Chain {
	return &Chain{carrierID: carrierID, stage: StageIdle, position: PositionUnknown, store: store}
}

// replayChain rebuilds a chain from its persisted events. Failed handovers
// never moved the position, so replaying only confirmed events keeps the
// last confirmed position intact. A carrier recovered mid-cycle is flagged
// for mandatory on-site confirmation.
func replayChain(carrierID string, store *ChainStore, events []ChainEvent) *Chain {
	chain := &Chain{carrierID: carrierID, stage: StageIdle, position: PositionUnknown, store: store, events: append([]ChainEvent(nil), events...)}
	for _, event := range events {
		switch event.Kind {
		case EventLoaded:
			chain.batch = event.Batch
			chain.runID = ""
			chain.runActive = false
			chain.layers = 0
			chain.inspected = false
			chain.stage = StageLoaded
			chain.position = PositionLoadPort
		case EventRunBound:
			chain.runID = event.RunID
			chain.runActive = true
			chain.stage = StageRunBound
		case EventChamberEntered:
			chain.position = PositionChamber
			chain.stage = StageChamberEntered
		case EventLayerCompleted:
			chain.layers = event.Layer
			chain.stage = StageLayerCompleted
		case EventInspected:
			chain.inspected = event.Passed
			if event.Passed {
				chain.stage = StageInspected
			}
		case EventHandover:
			chain.position = PositionUnload
			chain.runActive = false
			chain.stage = StageHandedOver
		case EventPositionConfirmed:
			chain.position = event.Position
		}
	}
	if chain.stage != StageIdle && chain.stage != StageHandedOver {
		chain.needsConfirm = true
	}
	return chain
}

func (c *Chain) appendLocked(event ChainEvent) (ChainEvent, error) {
	event.ID = model.NewID()
	event.CarrierID = c.carrierID
	event.At = time.Now().UTC()
	if c.store != nil {
		if err := c.store.Append(event); err != nil {
			return ChainEvent{}, err
		}
	}
	c.events = append(c.events, event)
	return event, nil
}

func (c *Chain) requireConfirmedLocked() error {
	if c.needsConfirm {
		return fmt.Errorf("carrier %s requires on-site position confirmation after recovery", c.carrierID)
	}
	return nil
}

// Load records a batch being loaded onto the carrier at the load port.
func (c *Chain) Load(batch string) (ChainEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requireConfirmedLocked(); err != nil {
		return ChainEvent{}, err
	}
	if batch == "" {
		return ChainEvent{}, fmt.Errorf("batch is required")
	}
	if c.stage != StageIdle && c.stage != StageHandedOver {
		return ChainEvent{}, fmt.Errorf("carrier %s has an active chain cycle at stage %s", c.carrierID, c.stage)
	}
	event, err := c.appendLocked(ChainEvent{Kind: EventLoaded, Batch: batch, Position: PositionLoadPort})
	if err != nil {
		return ChainEvent{}, err
	}
	c.batch = batch
	c.runID = ""
	c.runActive = false
	c.layers = 0
	c.inspected = false
	c.stage = StageLoaded
	c.position = PositionLoadPort
	return event, nil
}

// BindRun binds the carrier to an active run. A carrier can hold at most
// one active run binding; the binding is released only by handover.
func (c *Chain) BindRun(runID model.ID) (ChainEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requireConfirmedLocked(); err != nil {
		return ChainEvent{}, err
	}
	if runID == "" {
		return ChainEvent{}, fmt.Errorf("run id is required")
	}
	if c.runActive {
		return ChainEvent{}, fmt.Errorf("carrier %s is already bound to active run %s", c.carrierID, c.runID)
	}
	if c.stage != StageLoaded {
		return ChainEvent{}, fmt.Errorf("carrier %s is not waiting for a run binding (stage %s)", c.carrierID, c.stage)
	}
	event, err := c.appendLocked(ChainEvent{Kind: EventRunBound, RunID: runID, Position: c.position})
	if err != nil {
		return ChainEvent{}, err
	}
	c.runID = runID
	c.runActive = true
	c.stage = StageRunBound
	return event, nil
}

// EnterChamber records the carrier moving into the coating chamber.
func (c *Chain) EnterChamber() (ChainEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requireConfirmedLocked(); err != nil {
		return ChainEvent{}, err
	}
	if c.stage != StageRunBound {
		return ChainEvent{}, fmt.Errorf("carrier %s cannot enter chamber at stage %s", c.carrierID, c.stage)
	}
	event, err := c.appendLocked(ChainEvent{Kind: EventChamberEntered, RunID: c.runID, Position: PositionChamber})
	if err != nil {
		return ChainEvent{}, err
	}
	c.position = PositionChamber
	c.stage = StageChamberEntered
	return event, nil
}

// CompleteLayer records completion of layer number layer. Layer numbers
// must advance monotonically within a cycle.
func (c *Chain) CompleteLayer(layer int) (ChainEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requireConfirmedLocked(); err != nil {
		return ChainEvent{}, err
	}
	if c.stage != StageChamberEntered && c.stage != StageLayerCompleted {
		return ChainEvent{}, fmt.Errorf("carrier %s cannot complete layers at stage %s", c.carrierID, c.stage)
	}
	if layer < 1 {
		return ChainEvent{}, fmt.Errorf("layer number is required")
	}
	if layer <= c.layers {
		return ChainEvent{}, fmt.Errorf("layer %d already completed on carrier %s", layer, c.carrierID)
	}
	event, err := c.appendLocked(ChainEvent{Kind: EventLayerCompleted, RunID: c.runID, Position: c.position, Layer: layer})
	if err != nil {
		return ChainEvent{}, err
	}
	c.layers = layer
	c.stage = StageLayerCompleted
	return event, nil
}

// Inspect records an inspection result. A failed inspection keeps the
// carrier at layer_completed so it can be re-inspected.
func (c *Chain) Inspect(passed bool, detail string, operator string) (ChainEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requireConfirmedLocked(); err != nil {
		return ChainEvent{}, err
	}
	if c.stage != StageLayerCompleted {
		return ChainEvent{}, fmt.Errorf("carrier %s cannot be inspected at stage %s", c.carrierID, c.stage)
	}
	event, err := c.appendLocked(ChainEvent{Kind: EventInspected, RunID: c.runID, Position: c.position, Passed: passed, Operator: operator, Detail: detail})
	if err != nil {
		return ChainEvent{}, err
	}
	c.inspected = passed
	if passed {
		c.stage = StageInspected
	}
	return event, nil
}

// Handover unloads the carrier and releases the run binding. If the
// handover is not confirmed, the failure is recorded and the carrier
// stays at its last confirmed position with the run binding intact.
func (c *Chain) Handover(operator string, confirmed bool) (ChainEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requireConfirmedLocked(); err != nil {
		return ChainEvent{}, err
	}
	if c.stage != StageInspected {
		return ChainEvent{}, fmt.Errorf("carrier %s cannot be handed over at stage %s", c.carrierID, c.stage)
	}
	if !confirmed {
		event, err := c.appendLocked(ChainEvent{Kind: EventHandoverFailed, RunID: c.runID, Position: c.position, Operator: operator, Detail: "handover not confirmed"})
		if err != nil {
			return ChainEvent{}, err
		}
		return event, fmt.Errorf("handover failed; carrier %s remains at last confirmed position %s", c.carrierID, c.position)
	}
	event, err := c.appendLocked(ChainEvent{Kind: EventHandover, RunID: c.runID, Position: PositionUnload, Operator: operator})
	if err != nil {
		return ChainEvent{}, err
	}
	c.position = PositionUnload
	c.runActive = false
	c.stage = StageHandedOver
	return event, nil
}

// ConfirmPosition clears the recovery confirmation flag with a physically
// verified position. It is the only mutation allowed while needsConfirm
// is set, so recovery can never skip on-site confirmation.
func (c *Chain) ConfirmPosition(position Position, operator string) (ChainEvent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.needsConfirm {
		return ChainEvent{}, fmt.Errorf("carrier %s has no pending position confirmation", c.carrierID)
	}
	if position == "" || position == PositionUnknown {
		return ChainEvent{}, fmt.Errorf("confirmed position is required")
	}
	if operator == "" {
		return ChainEvent{}, fmt.Errorf("confirming operator is required")
	}
	event, err := c.appendLocked(ChainEvent{Kind: EventPositionConfirmed, RunID: c.runID, Position: position, Operator: operator})
	if err != nil {
		return ChainEvent{}, err
	}
	c.position = position
	c.needsConfirm = false
	return event, nil
}

func (c *Chain) Status() ChainStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := ChainStatus{CarrierID: c.carrierID, Batch: c.batch, RunID: c.runID, RunActive: c.runActive, Stage: c.stage, Position: c.position, LayersCompleted: c.layers, Inspected: c.inspected, NeedsConfirmation: c.needsConfirm}
	if len(c.events) > 0 {
		status.UpdatedAt = c.events[len(c.events)-1].At
	}
	return status
}

func (c *Chain) Events() []ChainEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ChainEvent(nil), c.events...)
}
