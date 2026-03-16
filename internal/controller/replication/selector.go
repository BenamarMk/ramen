// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HandlerSelector selects the appropriate replication handler based on the offloaded flag
type HandlerSelector struct {
	client    client.Client
	log       logr.Logger
	discovery *Discovery
}

// NewHandlerSelector creates a new handler selector
func NewHandlerSelector(client client.Client, log logr.Logger) *HandlerSelector {
	return &HandlerSelector{
		client:    client,
		log:       log,
		discovery: NewDiscovery(client, log),
	}
}

// SelectHandler selects the appropriate handler based on the StorageClass offloaded label
// - If offloaded = false (default): Use Legacy API (csi-addons VGR)
// - If offloaded = true: Use Neutral API (replication.storage.io)
func (s *HandlerSelector) SelectHandler(ctx context.Context, storageClassName string) (ReplicationHandler, HandlerType, error) {
	// Get the StorageClass
	storageClass, err := s.getStorageClass(ctx, storageClassName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get StorageClass %s: %w", storageClassName, err)
	}

	// Check if the StorageClass has the offloaded label
	offloaded := s.isOffloaded(storageClass)

	if offloaded {
		// Offloaded = true: Use Neutral API
		s.log.Info("Selecting Neutral API handler (offloaded=true)", "storageClass", storageClassName)
		
		// Check if neutral API is available
		neutralHandler := NewNeutralHandler()
		available, err := neutralHandler.IsAvailable(ctx, s.client)
		if err != nil {
			return nil, "", fmt.Errorf("failed to check neutral API availability: %w", err)
		}
		
		if !available {
			return nil, "", fmt.Errorf("neutral API not available but offloaded=true (StorageClass %s has offloaded label)", storageClassName)
		}
		
		return neutralHandler, NeutralHandlerType, nil
	}

	// Offloaded = false (default): Use Legacy API
	s.log.Info("Selecting Legacy API handler (offloaded=false or no offloaded label)", "storageClass", storageClassName)
	
	// Check if legacy API is available
	legacyHandler := NewLegacyHandler()
	available, err := legacyHandler.IsAvailable(ctx, s.client)
	if err != nil {
		return nil, "", fmt.Errorf("failed to check legacy API availability: %w", err)
	}
	
	if !available {
		return nil, "", fmt.Errorf("legacy API not available but offloaded=false")
	}
	
	return legacyHandler, LegacyHandlerType, nil
}

// SelectHandlerWithFallback selects the appropriate handler with fallback logic
// This provides backward compatibility by falling back to discovery if selection fails
func (s *HandlerSelector) SelectHandlerWithFallback(ctx context.Context, storageClassName string) (ReplicationHandler, HandlerType, error) {
	handler, handlerType, err := s.SelectHandler(ctx, storageClassName)
	if err != nil {
		s.log.Info("Handler selection failed, falling back to discovery", "error", err.Error(), "storageClass", storageClassName)
		
		// Fallback to discovery mechanism
		handler, handlerType, discErr := s.discovery.DiscoverHandler(ctx)
		if discErr != nil {
			return nil, "", fmt.Errorf("both handler selection and discovery failed: selection error: %w, discovery error: %v", err, discErr)
		}
		
		s.log.Info("Fallback to discovery successful", "type", handlerType)
		return handler, handlerType, nil
	}
	
	return handler, handlerType, nil
}

// getStorageClass retrieves a StorageClass by name
func (s *HandlerSelector) getStorageClass(ctx context.Context, name string) (*storagev1.StorageClass, error) {
	storageClass := &storagev1.StorageClass{}
	key := client.ObjectKey{Name: name}
	if err := s.client.Get(ctx, key, storageClass); err != nil {
		return nil, err
	}
	return storageClass, nil
}

// isOffloaded checks if a StorageClass has the offloaded label set to "true"
func (s *HandlerSelector) isOffloaded(sc *storagev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	
	labels := sc.GetLabels()
	if labels == nil {
		return false
	}
	
	value, exists := labels["ramendr.openshift.io/offloaded"]
	return exists && value == "true"
}

// Made with Bob
