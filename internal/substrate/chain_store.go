package substrate

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ChainStore persists carrier chain events to a JSONL file so chain
// state survives restarts and recovery replays from disk.
type ChainStore struct {
	mu   sync.Mutex
	path string
}

func NewChainStore(dir string) *ChainStore {
	if dir == "" {
		dir = "data"
	}
	_ = os.MkdirAll(dir, 0o755)
	return &ChainStore{path: filepath.Join(dir, "carrier_chain.jsonl")}
}

func (s *ChainStore) Append(event ChainEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

func (s *ChainStore) Events() ([]ChainEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return []ChainEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	result := make([]ChainEvent, 0)
	for scanner.Scan() {
		var event ChainEvent
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			result = append(result, event)
		}
	}
	return result, scanner.Err()
}
