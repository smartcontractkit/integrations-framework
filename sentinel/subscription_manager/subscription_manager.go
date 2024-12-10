// File: subscription_manager/subscription_manager.go
package subscription_manager

import (
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/smartcontractkit/chainlink-testing-framework/sentinel/api"
	"github.com/smartcontractkit/chainlink-testing-framework/sentinel/internal"

	"github.com/ethereum/go-ethereum/common"
)

type SubscriptionManagerConfig struct {
	Logger  *zerolog.Logger
	ChainID int64
}

// SubscriptionManager manages subscriptions for a specific chain.
type SubscriptionManager struct {
	registry          map[internal.EventKey][]chan api.Log
	registryMutex     sync.RWMutex
	logger            zerolog.Logger
	chainID           int64
	cachedEventKeys   []internal.EventKey
	cacheInitialized  bool
	cacheMutex        sync.RWMutex
	channelBufferSize int

	closing     bool       // Indicates if the manager is shutting down
	activeSends int        // Tracks active sends in BroadcastLog
	cond        *sync.Cond // Used to coordinate between BroadcastLog and Close
}

// NewSubscriptionManager initializes a new SubscriptionManager.
func NewSubscriptionManager(cfg SubscriptionManagerConfig) *SubscriptionManager {
	subscriptionManagerLogger := cfg.Logger.With().Str("component", "SubscriptionManager").Logger()
	mu := &sync.Mutex{}

	return &SubscriptionManager{
		registry:          make(map[internal.EventKey][]chan api.Log),
		logger:            subscriptionManagerLogger,
		chainID:           cfg.ChainID,
		channelBufferSize: 3,
		cond:              sync.NewCond(mu),
	}
}

// Subscribe registers a new subscription and returns a channel for events.
func (sm *SubscriptionManager) Subscribe(address common.Address, topic common.Hash) (chan api.Log, error) {
	if address == (common.Address{}) {
		sm.logger.Warn().Msg("Attempted to subscribe with an empty address")
		return nil, errors.New("address cannot be empty")
	}
	if topic == (common.Hash{}) {
		sm.logger.Warn().Msg("Attempted to subscribe with an empty topic")
		return nil, errors.New("topic cannot be empty")
	}

	sm.registryMutex.Lock()
	defer sm.registryMutex.Unlock()

	sm.invalidateCache()

	eventKey := internal.EventKey{Address: address, Topic: topic}
	newChan := make(chan api.Log, sm.channelBufferSize)
	sm.registry[eventKey] = append(sm.registry[eventKey], newChan)

	sm.logger.Info().
		Int64("ChainID", sm.chainID).
		Hex("Address", []byte(address.Hex())).
		Hex("Topic", []byte(topic.Hex())).
		Int64("SubscriberCount", int64(len(sm.registry[eventKey]))).
		Msg("New subscription added")

	return newChan, nil
}

// Unsubscribe removes a subscription and closes the channel.
func (sm *SubscriptionManager) Unsubscribe(address common.Address, topic common.Hash, ch chan api.Log) error {
	sm.registryMutex.Lock()
	defer sm.registryMutex.Unlock()

	eventKey := internal.EventKey{Address: address, Topic: topic}
	subscribers, exists := sm.registry[eventKey]
	if !exists {
		sm.logger.Warn().
			Int64("ChainID", sm.chainID).
			Hex("Address", []byte(address.Hex())).
			Hex("Topic", []byte(topic.Hex())).
			Msg("Attempted to unsubscribe from a non-existent EventKey")
		return errors.New("event key does not exist")
	}

	found := false // Flag to track if the subscriber was found

	for i, subscriber := range subscribers {
		if subscriber == ch {
			sm.invalidateCache()
			// Remove the subscriber from the list
			sm.registry[eventKey] = append(subscribers[:i], subscribers[i+1:]...)
			sm.logger.Info().
				Int64("ChainID", sm.chainID).
				Hex("Address", []byte(address.Hex())).
				Hex("Topic", []byte(topic.Hex())).
				Int64("RemainingSubscribers", int64(len(sm.registry[eventKey]))).
				Msg("Subscription removed")
			found = true
			break
		}
	}

	if !found {
		// Subscriber channel was not found in the registry
		sm.logger.Warn().
			Int64("ChainID", sm.chainID).
			Hex("Address", []byte(address.Hex())).
			Hex("Topic", []byte(topic.Hex())).
			Msg("Attempted to unsubscribe a non-existent subscriber")
		return errors.New("subscriber channel not found")
	}

	if len(sm.registry[eventKey]) == 0 {
		// Clean up the map if there are no more subscribers
		delete(sm.registry, eventKey)
		sm.logger.Debug().
			Int64("ChainID", sm.chainID).
			Hex("Address", []byte(address.Hex())).
			Hex("Topic", []byte(topic.Hex())).
			Msg("No remaining subscribers, removing EventKey from registry")
	}

	sm.cond.L.Lock()
	for sm.activeSends > 0 {
		sm.cond.Wait() // Wait for active broadcasts to complete
	}
	sm.cond.L.Unlock()

	close(ch) // Safely close the channel
	sm.logger.Info().
		Int64("ChainID", sm.chainID).
		Hex("Address", []byte(address.Hex())).
		Hex("Topic", []byte(topic.Hex())).
		Msg("Subscription removed")
	return nil
}

// BroadcastLog sends the log event to all relevant subscribers.
func (sm *SubscriptionManager) BroadcastLog(eventKey internal.EventKey, log api.Log) {
	sm.registryMutex.RLock()
	subscribers, exists := sm.registry[eventKey]
	sm.registryMutex.RUnlock()

	if !exists {
		sm.logger.Debug().
			Interface("EventKey", eventKey).
			Msg("EventKey not found in registry")
		return
	}

	var wg sync.WaitGroup
	for _, ch := range subscribers {
		sm.cond.L.Lock()
		if sm.closing {
			// If the manager is closing, skip sending logs
			defer sm.cond.L.Unlock()
			return
		}
		sm.activeSends++
		sm.cond.L.Unlock()
		wg.Add(1)
		go func(ch chan api.Log) {
			defer func() {
				sm.cond.L.Lock()
				sm.activeSends--
				sm.cond.Broadcast() // Notify Close() when all sends are done
				sm.cond.L.Unlock()
				wg.Done()
			}()
			select {
			case ch <- log:
			case <-time.After(100 * time.Millisecond): // Prevent blocking forever
				sm.logger.Warn().
					Int64("ChainID", sm.chainID).
					Msg("Log broadcast to channel timed out")
			}
		}(ch)
	}
	wg.Wait() // Wait for all sends to complete before returning
	sm.logger.Debug().
		Int64("ChainID", sm.chainID).
		Int("Subscribers", len(subscribers)).
		Hex("Address", []byte(eventKey.Address.Hex())).
		Hex("Topic", []byte(eventKey.Topic.Hex())).
		Msg("Log broadcasted to all subscribers")
}

// GetAddressesAndTopics retrieves all unique EventKeys.
// Implements caching: caches the result after the first call and invalidates it upon subscription changes.
// Returns a slice of EventKeys, each containing a unique address-topic pair.
func (sm *SubscriptionManager) GetAddressesAndTopics() []internal.EventKey {
	sm.cacheMutex.RLock()
	if sm.cacheInitialized {
		defer sm.cacheMutex.RUnlock()
		return sm.cachedEventKeys
	}
	sm.cacheMutex.RUnlock()

	sm.registryMutex.RLock()
	defer sm.registryMutex.RUnlock()

	eventKeys := make([]internal.EventKey, 0, len(sm.registry))
	for eventKey := range sm.registry {
		eventKeys = append(eventKeys, eventKey)
	}

	// Update the cache
	sm.cacheMutex.Lock()
	sm.cachedEventKeys = eventKeys
	sm.cacheInitialized = true
	sm.cacheMutex.Unlock()

	sm.logger.Debug().
		Int64("ChainID", sm.chainID).
		Int("UniqueEventKeys", len(sm.cachedEventKeys)).
		Msg("Cached EventKeys")

	return sm.cachedEventKeys
}

// invalidateCache invalidates the cached addresses and topics.
func (sm *SubscriptionManager) invalidateCache() {
	sm.cacheMutex.Lock()
	sm.cacheInitialized = false
	sm.cachedEventKeys = nil
	sm.cacheMutex.Unlock()

	sm.logger.Debug().
		Int64("ChainID", sm.chainID).
		Msg("Cache invalidated due to subscription change")
}

// Close gracefully shuts down the SubscriptionManager by closing all subscriber channels.
func (sm *SubscriptionManager) Close() {
	sm.registryMutex.Lock()
	sm.closing = true // Signal that the manager is closing
	sm.registryMutex.Unlock()

	// Wait for all active sends to complete
	sm.cond.L.Lock()
	for sm.activeSends > 0 {
		sm.cond.Wait()
	}
	sm.cond.L.Unlock()

	sm.registryMutex.Lock()
	defer sm.registryMutex.Unlock()

	for eventKey, subscribers := range sm.registry {
		for _, ch := range subscribers {
			close(ch)
		}
		delete(sm.registry, eventKey)
	}

	sm.invalidateCache()

	sm.logger.Info().
		Int64("ChainID", sm.chainID).
		Msg("SubscriptionManager closed, all subscriber channels have been closed")
}
