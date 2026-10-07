package substrate

import (
	"testing"

	"github.com/wyw14/cry-161/internal/model"
)

func advanceToInspected(t *testing.T, chain *Chain, batch string) model.ID {
	t.Helper()
	runID := model.NewID()
	if _, err := chain.Load(batch); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := chain.BindRun(runID); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := chain.EnterChamber(); err != nil {
		t.Fatalf("enter chamber: %v", err)
	}
	if _, err := chain.CompleteLayer(1); err != nil {
		t.Fatalf("complete layer: %v", err)
	}
	if _, err := chain.Inspect(true, "", "op"); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	return runID
}

func TestChainBindsSingleActiveRun(t *testing.T) {
	chain := NewChain("carrier-1", nil)
	runA, runB := model.NewID(), model.NewID()
	if _, err := chain.Load("batch-1"); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := chain.BindRun(runA); err != nil {
		t.Fatalf("bind run A: %v", err)
	}
	if _, err := chain.BindRun(runB); err == nil {
		t.Fatal("second bind must fail while run A is active")
	}
	if status := chain.Status(); status.RunID != runA || !status.RunActive {
		t.Fatalf("binding must stay with run A, got %+v", status)
	}
	if _, err := chain.EnterChamber(); err != nil {
		t.Fatalf("enter chamber: %v", err)
	}
	if _, err := chain.CompleteLayer(1); err != nil {
		t.Fatalf("complete layer: %v", err)
	}
	if _, err := chain.Inspect(true, "", "op"); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if _, err := chain.Handover("op", true); err != nil {
		t.Fatalf("handover: %v", err)
	}
	if _, err := chain.Load("batch-2"); err != nil {
		t.Fatalf("reload after handover: %v", err)
	}
	if _, err := chain.BindRun(runB); err != nil {
		t.Fatalf("bind run B after handover: %v", err)
	}
}

func TestChainHandoverFailurePreservesPosition(t *testing.T) {
	chain := NewChain("carrier-1", nil)
	runID := advanceToInspected(t, chain, "batch-1")
	if _, err := chain.Handover("op", false); err == nil {
		t.Fatal("unconfirmed handover must fail")
	}
	status := chain.Status()
	if status.Position != PositionChamber {
		t.Fatalf("failed handover must keep last confirmed position %s, got %s", PositionChamber, status.Position)
	}
	if !status.RunActive || status.RunID != runID {
		t.Fatal("failed handover must keep the active run binding")
	}
	if status.Stage != StageInspected {
		t.Fatalf("failed handover must not advance stage, got %s", status.Stage)
	}
	events := chain.Events()
	if events[len(events)-1].Kind != EventHandoverFailed {
		t.Fatalf("failed handover must be recorded, got %s", events[len(events)-1].Kind)
	}
	if _, err := chain.Handover("op", true); err != nil {
		t.Fatalf("confirmed handover: %v", err)
	}
	status = chain.Status()
	if status.Position != PositionUnload || status.RunActive {
		t.Fatalf("confirmed handover must unload and release the run, got %+v", status)
	}
}

func TestChainRecoveryRequiresOnSiteConfirmation(t *testing.T) {
	dir := t.TempDir()
	chain := NewChain("carrier-1", NewChainStore(dir))
	advanceToInspected(t, chain, "batch-1")
	if _, err := chain.Handover("op", false); err == nil {
		t.Fatal("unconfirmed handover must fail")
	}

	registry := NewChainRegistry(NewChainStore(dir))
	if err := registry.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	recovered, ok := registry.Get("carrier-1")
	if !ok {
		t.Fatal("recovered registry must contain carrier-1")
	}
	status := recovered.Status()
	if !status.NeedsConfirmation {
		t.Fatal("mid-cycle recovery must require on-site confirmation")
	}
	if status.Position != PositionChamber {
		t.Fatalf("failed handover must not move recovered position, got %s", status.Position)
	}
	if !status.RunActive {
		t.Fatal("recovered carrier must keep the active run binding")
	}
	if _, err := recovered.Handover("op", true); err == nil {
		t.Fatal("recovered carrier must not advance before on-site confirmation")
	}
	if _, err := recovered.ConfirmPosition(PositionChamber, ""); err == nil {
		t.Fatal("confirmation without operator must fail")
	}
	if _, err := recovered.ConfirmPosition(PositionChamber, "op"); err != nil {
		t.Fatalf("confirm position: %v", err)
	}
	if _, err := recovered.Handover("op", true); err != nil {
		t.Fatalf("handover after confirmation: %v", err)
	}
}

func TestChainRecoveredAfterHandoverNeedsNoConfirmation(t *testing.T) {
	dir := t.TempDir()
	chain := NewChain("carrier-1", NewChainStore(dir))
	advanceToInspected(t, chain, "batch-1")
	if _, err := chain.Handover("op", true); err != nil {
		t.Fatalf("handover: %v", err)
	}

	registry := NewChainRegistry(NewChainStore(dir))
	if err := registry.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	recovered, ok := registry.Get("carrier-1")
	if !ok {
		t.Fatal("recovered registry must contain carrier-1")
	}
	status := recovered.Status()
	if status.NeedsConfirmation {
		t.Fatal("handed-over carrier must not require confirmation")
	}
	if status.Position != PositionUnload || status.Stage != StageHandedOver {
		t.Fatalf("recovered carrier must be handed over at unload, got %+v", status)
	}
	if _, err := recovered.Load("batch-2"); err != nil {
		t.Fatalf("recovered carrier must accept a new cycle: %v", err)
	}
}
