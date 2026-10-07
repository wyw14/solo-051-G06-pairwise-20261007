package substrate

import (
	"fmt"
	"github.com/wyw14/cry-161/internal/model"
	"sort"
	"sync"
)

// ChainRegistry manages all carriers on the line and rebuilds their
// chains from the persisted event log on recovery.
type ChainRegistry struct {
	mu     sync.RWMutex
	chains map[string]*Chain
	store  *ChainStore
}

func NewChainRegistry(store *ChainStore) *ChainRegistry {
	return &ChainRegistry{chains: map[string]*Chain{}, store: store}
}

func (r *ChainRegistry) Get(carrierID string) (*Chain, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	chain, ok := r.chains[carrierID]
	return chain, ok
}

// GetOrLoad returns the chain for carrierID, creating an idle one if the
// carrier has no recorded history.
func (r *ChainRegistry) GetOrLoad(carrierID string) (*Chain, error) {
	r.mu.RLock()
	chain, ok := r.chains[carrierID]
	r.mu.RUnlock()
	if ok {
		return chain, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if chain, ok := r.chains[carrierID]; ok {
		return chain, nil
	}
	events, err := r.store.Events()
	if err != nil {
		return nil, err
	}
	chain = replayChain(carrierID, r.store, filterChainEvents(events, carrierID))
	r.chains[carrierID] = chain
	return chain, nil
}

// Recover rebuilds every carrier chain from the persisted event log.
// Carriers recovered mid-cycle require on-site position confirmation
// before they can advance; nothing is trusted from memory.
func (r *ChainRegistry) Recover() error {
	events, err := r.store.Events()
	if err != nil {
		return fmt.Errorf("carrier chain recovery: %w", err)
	}
	byCarrier := map[string][]ChainEvent{}
	for _, event := range events {
		byCarrier[event.CarrierID] = append(byCarrier[event.CarrierID], event)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for carrierID, carrierEvents := range byCarrier {
		r.chains[carrierID] = replayChain(carrierID, r.store, carrierEvents)
	}
	return nil
}

func (r *ChainRegistry) List() []ChainStatus {
	r.mu.RLock()
	chains := make([]*Chain, 0, len(r.chains))
	for _, chain := range r.chains {
		chains = append(chains, chain)
	}
	r.mu.RUnlock()
	result := make([]ChainStatus, 0, len(chains))
	for _, chain := range chains {
		result = append(result, chain.Status())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CarrierID < result[j].CarrierID })
	return result
}

// ActiveBinding returns the carrier currently bound to runID, if any.
func (r *ChainRegistry) ActiveBinding(runID model.ID) (ChainStatus, bool) {
	for _, status := range r.List() {
		if status.RunActive && status.RunID == runID {
			return status, true
		}
	}
	return ChainStatus{}, false
}

func filterChainEvents(events []ChainEvent, carrierID string) []ChainEvent {
	result := make([]ChainEvent, 0)
	for _, event := range events {
		if event.CarrierID == carrierID {
			result = append(result, event)
		}
	}
	return result
}
