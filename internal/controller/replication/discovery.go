// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Discovery handles the automatic detection of available replication APIs
type Discovery struct {
	client  client.Client
	log     logr.Logger
	neutral *NeutralHandler
	legacy  *LegacyHandler
}

// NewDiscovery creates a new Discovery instance
func NewDiscovery(client client.Client, log logr.Logger) *Discovery {
	return &Discovery{
		client:  client,
		log:     log,
		neutral: NewNeutralHandler(),
		legacy:  NewLegacyHandler(),
	}
}

// DiscoverHandler discovers and returns the appropriate ReplicationHandler
// Priority: Neutral API > Legacy API
// This implements the "Bridge" pattern for smooth transition
func (d *Discovery) DiscoverHandler(ctx context.Context) (ReplicationHandler, HandlerType, error) {
	// Priority 1: Check for neutral API (replication.storage.io)
	neutralAvailable, err := d.neutral.IsAvailable(ctx, d.client)
	if err != nil {
		d.log.Error(err, "Failed to check neutral API availability")
	} else if neutralAvailable {
		d.log.Info("Discovered neutral replication API", "apiGroup", NeutralAPIGroup)
		return d.neutral, NeutralHandlerType, nil
	}

	// Priority 2: Fallback to legacy API (replication.storage.openshift.io)
	legacyAvailable, err := d.legacy.IsAvailable(ctx, d.client)
	if err != nil {
		d.log.Error(err, "Failed to check legacy API availability")
		return nil, "", fmt.Errorf("failed to check legacy API: %w", err)
	}

	if legacyAvailable {
		d.log.Info("Discovered legacy replication API", "apiGroup", LegacyAPIGroup)
		return d.legacy, LegacyHandlerType, nil
	}

	// No replication API available
	return nil, "", fmt.Errorf("no replication API available (checked %s and %s)",
		NeutralAPIGroup, LegacyAPIGroup)
}

// DiscoverHandlerForPeerClass discovers the handler based on PeerClass configuration
// This allows for per-PeerClass API selection during the transition period
func (d *Discovery) DiscoverHandlerForPeerClass(
	ctx context.Context,
	peerClassName string,
	preferNeutral bool,
) (ReplicationHandler, HandlerType, error) {
	// If explicitly preferring neutral, try it first
	if preferNeutral {
		neutralAvailable, err := d.neutral.IsAvailable(ctx, d.client)
		if err == nil && neutralAvailable {
			d.log.Info("Using neutral API for PeerClass",
				"peerClass", peerClassName,
				"apiGroup", NeutralAPIGroup)
			return d.neutral, NeutralHandlerType, nil
		}
	}

	// Otherwise use standard discovery
	return d.DiscoverHandler(ctx)
}

// GetNeutralHandler returns the neutral handler (for testing or explicit use)
func (d *Discovery) GetNeutralHandler() *NeutralHandler {
	return d.neutral
}

// GetLegacyHandler returns the legacy handler (for testing or explicit use)
func (d *Discovery) GetLegacyHandler() *LegacyHandler {
	return d.legacy
}

// IsNeutralAPIAvailable checks if the neutral API is available
func (d *Discovery) IsNeutralAPIAvailable(ctx context.Context) (bool, error) {
	return d.neutral.IsAvailable(ctx, d.client)
}

// IsLegacyAPIAvailable checks if the legacy API is available
func (d *Discovery) IsLegacyAPIAvailable(ctx context.Context) (bool, error) {
	return d.legacy.IsAvailable(ctx, d.client)
}

// GetAvailableAPIs returns information about which APIs are available
func (d *Discovery) GetAvailableAPIs(ctx context.Context) (map[string]bool, error) {
	apis := make(map[string]bool)

	neutralAvailable, err := d.neutral.IsAvailable(ctx, d.client)
	if err != nil {
		d.log.Error(err, "Failed to check neutral API")
	}
	apis[NeutralAPIGroup] = neutralAvailable

	legacyAvailable, err := d.legacy.IsAvailable(ctx, d.client)
	if err != nil {
		d.log.Error(err, "Failed to check legacy API")
	}
	apis[LegacyAPIGroup] = legacyAvailable

	return apis, nil
}

// HandlerFactory creates handlers based on explicit type
type HandlerFactory struct{}

// NewHandlerFactory creates a new HandlerFactory
func NewHandlerFactory() *HandlerFactory {
	return &HandlerFactory{}
}

// CreateHandler creates a handler of the specified type
func (f *HandlerFactory) CreateHandler(handlerType HandlerType) (ReplicationHandler, error) {
	switch handlerType {
	case NeutralHandlerType:
		return NewNeutralHandler(), nil
	case LegacyHandlerType:
		return NewLegacyHandler(), nil
	default:
		return nil, fmt.Errorf("unknown handler type: %s", handlerType)
	}
}

// CreateNeutralHandler creates a neutral handler
func (f *HandlerFactory) CreateNeutralHandler() *NeutralHandler {
	return NewNeutralHandler()
}

// CreateLegacyHandler creates a legacy handler
func (f *HandlerFactory) CreateLegacyHandler() *LegacyHandler {
	return NewLegacyHandler()
}

// Made with Bob
