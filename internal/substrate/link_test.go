package substrate

import (
	"testing"

	"github.com/wyw14/cry-161/internal/journal"
	"github.com/wyw14/cry-161/internal/model"
)

func newTestLink(t *testing.T) (*Link, string) {
	t.Helper()
	dir := t.TempDir()
	store := journal.New(dir)
	return NewLink(store), dir
}

func prepBoundCarrier(t *testing.T, link *Link, carrierID string, runID model.ID) {
	t.Helper()
	if _, err := link.LoadBatch(carrierID, "batch-1", "load_port", "op"); err != nil {
		t.Fatalf("load batch: %v", err)
	}
	if _, err := link.BindRun(carrierID, runID, "load_port", "op"); err != nil {
		t.Fatalf("bind run: %v", err)
	}
}

// A carrier is bound to at most one active run, and a run to at most one carrier.
func TestExclusiveRunBinding(t *testing.T) {
	link, _ := newTestLink(t)
	runA := model.NewID()
	runB := model.NewID()

	prepBoundCarrier(t, link, "c1", runA)

	// Same carrier cannot bind a second run.
	if _, err := link.BindRun("c1", runB, "load_port", "op"); err == nil {
		t.Fatal("expected error binding a second run to one carrier")
	}

	// A second carrier cannot steal a run already bound elsewhere.
	if _, err := link.LoadBatch("c2", "batch-2", "load_port", "op"); err != nil {
		t.Fatalf("load batch c2: %v", err)
	}
	if _, err := link.BindRun("c2", runA, "load_port", "op"); err == nil {
		t.Fatal("expected error binding an already-bound run to another carrier")
	}

	if holder, ok := link.ActiveCarrier(runA); !ok || holder != "c1" {
		t.Fatalf("runA should remain held by c1, got %q ok=%t", holder, ok)
	}
}

func TestFullChain(t *testing.T) {
	link, _ := newTestLink(t)
	runID := model.NewID()
	prepBoundCarrier(t, link, "c1", runID)

	if _, err := link.EnterChamber("c1", runID, "coat-01", "op"); err != nil {
		t.Fatalf("enter chamber: %v", err)
	}
	if _, err := link.LayerComplete("c1", runID, 1); err != nil {
		t.Fatalf("layer complete: %v", err)
	}
	if _, err := link.Inspect("c1", runID, "visual", true, "op"); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	state, err := link.Unload("c1", runID, "coat-01", "op")
	if err != nil {
		t.Fatalf("unload: %v", err)
	}
	if state.Stage != model.StageUnloaded || state.Active {
		t.Fatalf("unexpected terminal state: %+v", state)
	}
	if _, ok := link.ActiveCarrier(runID); ok {
		t.Fatal("run binding should be released after unload")
	}
}

// Out-of-order stages must be rejected.
func TestStageOrdering(t *testing.T) {
	link, _ := newTestLink(t)
	runID := model.NewID()

	// No batch loaded -> cannot bind.
	if _, err := link.BindRun("ghost", runID, "load_port", "op"); err == nil {
		t.Fatal("expected error binding without a loaded batch")
	}

	prepBoundCarrier(t, link, "c1", runID)
	// Layer completion before chamber entry.
	if _, err := link.LayerComplete("c1", runID, 1); err == nil {
		t.Fatal("expected error completing a layer before chamber entry")
	}
	// Inspection before any layer.
	if _, err := link.Inspect("c1", runID, "visual", true, "op"); err == nil {
		t.Fatal("expected error inspecting before layer completion")
	}
	// Unload before inspection.
	if _, err := link.Unload("c1", runID, "", "op"); err == nil {
		t.Fatal("expected error unloading before inspection")
	}
}

// A failed handoff keeps the carrier at its last confirmed position and blocks
// motion until an operator confirms the carrier on site.
func TestHandoffFailureKeepsPosition(t *testing.T) {
	link, _ := newTestLink(t)
	runID := model.NewID()
	prepBoundCarrier(t, link, "c1", runID)
	if _, err := link.EnterChamber("c1", runID, "coat-01", "op"); err != nil {
		t.Fatalf("enter chamber: %v", err)
	}

	handoff, err := link.FailHandoff("c1", runID, "unload_port", "robot timeout")
	if err != nil {
		t.Fatalf("fail handoff: %v", err)
	}
	if handoff.From != "coat-01" || handoff.To != "unload_port" || handoff.Confirmed {
		t.Fatalf("unexpected handoff: %+v", handoff)
	}

	state, _ := link.State("c1")
	if state.Position.Site != "coat-01" || !state.Position.Confirmed {
		t.Fatalf("carrier must stay at last confirmed position, got %+v", state.Position)
	}
	if state.Pending == nil || state.Pending.To != "unload_port" {
		t.Fatal("expected a pending handoff to unload_port")
	}

	// No motion is allowed while the handoff is unresolved.
	if _, err := link.EnterChamber("c1", runID, "coat-02", "op"); err == nil {
		t.Fatal("expected chamber entry to be blocked after failed handoff")
	}
	if _, err := link.Unload("c1", runID, "unload_port", "op"); err == nil {
		t.Fatal("expected unload to be blocked after failed handoff")
	}

	// Confirming at the wrong site is rejected: memory cannot relocate the carrier.
	if _, err := link.ConfirmSite("c1", runID, "unload_port", "op"); err == nil {
		t.Fatal("expected confirmation at the unreported destination to be rejected")
	}

	// Operator physically confirms the carrier still at the last site.
	state, err = link.ConfirmSite("c1", runID, "coat-01", "op")
	if err != nil {
		t.Fatalf("confirm site: %v", err)
	}
	if state.Pending != nil || !state.Position.Confirmed {
		t.Fatalf("hold should be cleared after site confirmation: %+v", state)
	}
}

// ConfirmHandoff only succeeds when the reported site matches the destination.
func TestConfirmHandoffRequiresMatchingSite(t *testing.T) {
	link, _ := newTestLink(t)
	runID := model.NewID()
	prepBoundCarrier(t, link, "c1", runID)
	if _, err := link.EnterChamber("c1", runID, "coat-01", "op"); err != nil {
		t.Fatalf("enter chamber: %v", err)
	}
	if _, err := link.FailHandoff("c1", runID, "unload_port", "x"); err != nil {
		t.Fatalf("fail handoff: %v", err)
	}
	if _, err := link.ConfirmHandoff("c1", runID, "coat-01", "op"); err == nil {
		t.Fatal("expected confirmation at wrong site to fail")
	}
	state, err := link.ConfirmHandoff("c1", runID, "unload_port", "op")
	if err != nil {
		t.Fatalf("confirm handoff at destination: %v", err)
	}
	if state.Position.Site != "unload_port" || !state.Position.Confirmed {
		t.Fatalf("carrier should now be confirmed at destination: %+v", state.Position)
	}
}

// After recovery, journal replay reconstructs state but forces fresh on-site
// confirmation; in-memory state alone must not allow motion to be skipped.
func TestResumeRequiresSiteConfirmation(t *testing.T) {
	dir := t.TempDir()
	store := journal.New(dir)
	runID := model.NewID()

	func() {
		link := NewLink(store)
		prepBoundCarrier(t, link, "c1", runID)
		if _, err := link.EnterChamber("c1", runID, "coat-01", "op"); err != nil {
			t.Fatalf("enter chamber: %v", err)
		}
		if _, err := link.LayerComplete("c1", runID, 1); err != nil {
			t.Fatalf("layer complete: %v", err)
		}
	}()

	// Fresh process: replay the durable journal.
	resumed := NewLink(journal.New(dir))
	if err := resumed.Replay(); err != nil {
		t.Fatalf("replay: %v", err)
	}
	state, ok := resumed.State("c1")
	if !ok {
		t.Fatal("carrier should be reconstructed from the journal")
	}
	if state.Stage != model.StageLayerDone || state.RunID != runID {
		t.Fatalf("replayed state mismatch: %+v", state)
	}
	if !state.NeedsConfirmation || state.Position.Confirmed {
		t.Fatal("resumed active carrier must require fresh site confirmation")
	}

	// Motion is blocked purely on the strength of remembered state.
	if _, err := resumed.LayerComplete("c1", runID, 2); err == nil {
		t.Fatal("expected layer completion to be blocked pending site confirmation")
	}
	if _, err := resumed.FailHandoff("c1", runID, "unload_port", "x"); err == nil {
		t.Fatal("expected handoff to be blocked pending site confirmation")
	}

	// Confirming a site the journal never recorded is rejected.
	if _, err := resumed.ConfirmSite("c1", runID, "somewhere-else", "op"); err == nil {
		t.Fatal("expected confirmation at an unrecorded site to fail")
	}

	// Positive on-site confirmation at the last known position unlocks motion.
	state, err := resumed.ConfirmSite("c1", runID, "coat-01", "op")
	if err != nil {
		t.Fatalf("confirm site: %v", err)
	}
	if state.NeedsConfirmation || !state.Position.Confirmed {
		t.Fatalf("confirmation should clear the hold: %+v", state)
	}
	if _, err := resumed.LayerComplete("c1", runID, 2); err != nil {
		t.Fatalf("layer completion after confirmation: %v", err)
	}
}

// A completed (unloaded) carrier is not forced through confirmation on replay.
func TestResumeUnloadedCarrierNotBlocked(t *testing.T) {
	dir := t.TempDir()
	runID := model.NewID()
	func() {
		link := NewLink(journal.New(dir))
		prepBoundCarrier(t, link, "c1", runID)
		if _, err := link.EnterChamber("c1", runID, "coat-01", "op"); err != nil {
			t.Fatalf("enter chamber: %v", err)
		}
		if _, err := link.LayerComplete("c1", runID, 1); err != nil {
			t.Fatalf("layer: %v", err)
		}
		if _, err := link.Inspect("c1", runID, "visual", true, "op"); err != nil {
			t.Fatalf("inspect: %v", err)
		}
		if _, err := link.Unload("c1", runID, "coat-01", "op"); err != nil {
			t.Fatalf("unload: %v", err)
		}
	}()
	resumed := NewLink(journal.New(dir))
	if err := resumed.Replay(); err != nil {
		t.Fatalf("replay: %v", err)
	}
	state, _ := resumed.State("c1")
	if state.Active || state.NeedsConfirmation {
		t.Fatalf("unloaded carrier should be inactive and need no confirmation: %+v", state)
	}
}
