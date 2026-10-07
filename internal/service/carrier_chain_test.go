package service

import (
	"context"
	"testing"

	"github.com/wyw14/cry-161/internal/model"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	t.Setenv("OPTICCOAT_DATA", t.TempDir())
	return New()
}

func TestBindCarrierRunRequiresActiveRun(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.BindCarrierRun("carrier-1", model.NewID()); err == nil {
		t.Fatal("binding an unknown run must fail")
	}
	recipes := svc.Recipes.List()
	if len(recipes) == 0 {
		t.Fatal("expected a seeded recipe")
	}
	value, err := svc.Start(context.Background(), "batch-1", recipes[0].ID)
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := svc.LoadCarrier("carrier-1", "batch-1"); err != nil {
		t.Fatalf("load carrier: %v", err)
	}
	status, err := svc.BindCarrierRun("carrier-1", value.ID)
	if err != nil {
		t.Fatalf("bind active run: %v", err)
	}
	if !status.RunActive || status.RunID != value.ID {
		t.Fatalf("expected active binding to run %s, got %+v", value.ID, status)
	}
	if _, err := svc.LoadCarrier("carrier-2", "batch-1"); err != nil {
		t.Fatalf("load second carrier: %v", err)
	}
	if _, err := svc.BindCarrierRun("carrier-2", value.ID); err == nil {
		t.Fatal("run already bound to carrier-1 must not bind to carrier-2")
	}
	svc.Runs.UpdateError(value.ID, nil)
	if _, err := svc.BindCarrierRun("carrier-2", value.ID); err == nil {
		t.Fatal("binding a failed run must fail")
	}
}

func TestCarrierChainPersistsAcrossServiceRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPTICCOAT_DATA", dir)
	first := New()
	if _, err := first.LoadCarrier("carrier-1", "batch-1"); err != nil {
		t.Fatalf("load carrier: %v", err)
	}
	recipes := first.Recipes.List()
	value, err := first.Start(context.Background(), "batch-1", recipes[0].ID)
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := first.BindCarrierRun("carrier-1", value.ID); err != nil {
		t.Fatalf("bind run: %v", err)
	}

	second := New()
	status, _, err := second.CarrierChain("carrier-1")
	if err != nil {
		t.Fatalf("load recovered chain: %v", err)
	}
	if !status.NeedsConfirmation {
		t.Fatal("restarted service must require on-site confirmation")
	}
	if _, err := second.CarrierEnterChamber("carrier-1"); err == nil {
		t.Fatal("recovered carrier must not advance before on-site confirmation")
	}
	if _, err := second.ConfirmCarrierPosition("carrier-1", "load_port", "op"); err != nil {
		t.Fatalf("confirm position: %v", err)
	}
	if _, err := second.CarrierEnterChamber("carrier-1"); err != nil {
		t.Fatalf("enter chamber after confirmation: %v", err)
	}
}
