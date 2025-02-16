package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cedar-policy/cedar-go"
	"k8s.io/klog/v2"
)

// policyStore contains the Indexers that stores policies
type localPolicyStore struct {
	// TODO use a sync.Once?
	initalPolicyLoadComplete   bool
	initalPolicyLoadCompleteMu sync.RWMutex

	name            string
	directory       string
	refreshInterval time.Duration
	policies        *cedar.PolicySet
	policiesMu      sync.RWMutex
}

// NewPolicyStore creates a PolicyStore
func NewLocalPolicyStore(directory string, refreshInterval time.Duration) PolicyStore {
	// TODO: impose some validation (positive, min/max) on refreshInterval
	// TODO: return an error if directory doesn't exist at startup
	store := &localPolicyStore{
		directory:       directory,
		refreshInterval: refreshInterval,
		name:            "FilePolicyStore",
	}
	store.loadPolicies()
	go store.reloadAsync()
	return store
}

func (s *localPolicyStore) reloadAsync() {
	ticker := time.NewTicker(s.refreshInterval)
	for range ticker.C {
		s.loadPolicies()
	}
}

func (s *localPolicyStore) loadPolicies() {
	s.initalPolicyLoadCompleteMu.Lock()
	s.initalPolicyLoadComplete = true
	defer s.initalPolicyLoadCompleteMu.Unlock()

	files, err := os.ReadDir(s.directory)
	if err != nil {
		klog.Errorf("Error reading policy directory: %v", err)
		return
	}

	s.policiesMu.Lock()
	defer s.policiesMu.Unlock()
	policySet := cedar.NewPolicySet()
	for _, file := range files {
		if file.IsDir() || !file.Type().IsRegular() {
			klog.V(6).InfoS("Skipping non-regular or directory file", "file", file.Name())
			continue
		}
		if filepath.Ext(file.Name()) != ".cedar" {
			klog.V(6).InfoS("Skipping non-cedar file", "file", file.Name())
			continue
		}
		policySetFile := filepath.Join(s.directory, file.Name())

		data, err := os.ReadFile(policySetFile)
		if err != nil {
			klog.Errorf("Error reading policy file: %v", err)
			continue
		}

		policySlice, err := cedar.NewPolicyListFromBytes(file.Name(), data)
		if err != nil {
			klog.Errorf("Error loading policy file: %v", err)
			continue
		}

		for i, p := range policySlice {
			policyID := cedar.PolicyID(fmt.Sprintf("%s.policy%d", file.Name(), i))
			policySet.Store(policyID, p)
		}
	}

	s.policies = policySet
}

func (s *localPolicyStore) PolicySet(_ context.Context) *cedar.PolicySet {
	s.policiesMu.RLock()
	defer s.policiesMu.RUnlock()
	return s.policies
}

func (s *localPolicyStore) InitalPolicyLoadComplete() bool {
	s.initalPolicyLoadCompleteMu.RLock()
	defer s.initalPolicyLoadCompleteMu.RUnlock()
	return s.initalPolicyLoadComplete
}

func (s *localPolicyStore) Name() string {
	return s.name
}
