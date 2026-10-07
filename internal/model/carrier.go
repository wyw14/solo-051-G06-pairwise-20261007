package model

import "time"

// CarrierStage is a stage in the substrate carrier chain of custody.
type CarrierStage string

const (
	StageBatchLoaded CarrierStage = "batch_loaded"
	StageRunBound    CarrierStage = "run_bound"
	StageChamberIn   CarrierStage = "chamber_entered"
	StageLayerDone   CarrierStage = "layer_completed"
	StageInspected   CarrierStage = "inspected"
	StageUnloaded    CarrierStage = "unloaded"
)

// Carrier event kinds persisted to the journal. The journal is the durable
// record of custody; in-memory state is reconstructed from these events.
const (
	CarrierBatchLoadedEvent      = "carrier.batch_loaded"
	CarrierRunBoundEvent         = "carrier.run_bound"
	CarrierChamberEnteredEvent   = "carrier.chamber_entered"
	CarrierLayerCompletedEvent   = "carrier.layer_completed"
	CarrierInspectedEvent        = "carrier.inspected"
	CarrierUnloadedEvent         = "carrier.unloaded"
	CarrierHandoffConfirmedEvent = "carrier.handoff_confirmed"
	CarrierHandoffFailedEvent    = "carrier.handoff_failed"
	CarrierSiteConfirmedEvent    = "carrier.site_confirmed"
)

// CarrierPosition is a confirmed physical location of a carrier along the line.
type CarrierPosition struct {
	Site        string    `json:"site"`
	Confirmed   bool      `json:"confirmed"`
	ConfirmedBy string    `json:"confirmed_by,omitempty"`
	At          time.Time `json:"at"`
}

// CarrierHandoff records a transfer between two sites. A failed handoff keeps
// the carrier at the last confirmed position — the target is never accepted
// without positive confirmation.
type CarrierHandoff struct {
	ID          ID        `json:"id"`
	CarrierID   string    `json:"carrier_id"`
	RunID       ID        `json:"run_id,omitempty"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Confirmed   bool      `json:"confirmed"`
	Detail      string    `json:"detail,omitempty"`
	At          time.Time `json:"at"`
	ConfirmedAt time.Time `json:"confirmed_at,omitempty"`
}

// CarrierState is the durable custody state of one carrier.
type CarrierState struct {
	CarrierID         string          `json:"carrier_id"`
	Batch             string          `json:"batch,omitempty"`
	RunID             ID              `json:"run_id,omitempty"`
	Stage             CarrierStage    `json:"stage"`
	Active            bool            `json:"active"`
	LayersDone        int             `json:"layers_done"`
	Inspection        string          `json:"inspection,omitempty"`
	InspectionOK      bool            `json:"inspection_ok"`
	Position          CarrierPosition `json:"position"`
	Pending           *CarrierHandoff `json:"pending_handoff,omitempty"`
	NeedsConfirmation bool            `json:"needs_confirmation"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// CarrierEvent converts a persisted journal event into a typed carrier event.
type CarrierEvent struct {
	Kind  string
	Event Event
}

var carrierStageRank = map[CarrierStage]int{
	StageBatchLoaded: 1,
	StageRunBound:    2,
	StageChamberIn:   3,
	StageLayerDone:   4,
	StageInspected:   5,
	StageUnloaded:    6,
}

// StageAtLeast reports whether stage has reached or passed want.
func StageAtLeast(stage CarrierStage, want CarrierStage) bool {
	return carrierStageRank[stage] >= carrierStageRank[want]
}
