// File: subscription_manager/subscription_manager_test.go
package subscription_manager

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/smartcontractkit/chainlink-testing-framework/lib/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/sentinel/api"
	"github.com/smartcontractkit/chainlink-testing-framework/sentinel/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSubscriptionManager initializes a SubscriptionManager with a MockLogger for testing.
func setupSubscriptionManager(t *testing.T) *SubscriptionManager {
	testLogger := logging.GetTestLogger(t)
	return NewSubscriptionManager(SubscriptionManagerConfig{Logger: &testLogger, ChainID: 1})
}

func TestSubscriptionManager_Subscribe(t *testing.T) {
	manager := setupSubscriptionManager(t)

	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")

	// Valid subscription
	ch, err := manager.Subscribe(address, topic)
	require.NoError(t, err)
	assert.NotNil(t, ch)

	// Invalid subscription with empty address
	_, err = manager.Subscribe(common.Address{}, topic)
	assert.Error(t, err)

	// Invalid subscription with empty topic
	_, err = manager.Subscribe(address, common.Hash{})
	assert.Error(t, err)

	// Check registry state
	manager.registryMutex.RLock()
	defer manager.registryMutex.RUnlock()
	assert.Len(t, manager.registry, 1, "Registry should contain one event key")
	assert.Len(t, manager.registry[internal.EventKey{Address: address, Topic: topic}], 1, "EventKey should have one subscriber")
}

func TestSubscriptionManager_MultipleSubscribers(t *testing.T) {
	manager := setupSubscriptionManager(t)

	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	eventKey := internal.EventKey{Address: address, Topic: topic}

	// Subscribe first consumer
	ch1, err := manager.Subscribe(address, topic)
	require.NoError(t, err)

	// Subscribe second consumer
	ch2, err := manager.Subscribe(address, topic)
	require.NoError(t, err)

	// Verify that the list of channels grows
	manager.registryMutex.RLock()
	subscribers := manager.registry[eventKey]
	manager.registryMutex.RUnlock()
	assert.Len(t, subscribers, 2, "There should be two channels subscribed to the EventKey")

	// Broadcast a log and ensure both channels receive it
	logEvent := api.Log{
		BlockNumber: 1,
		TxHash:      common.HexToHash("0x1234"),
		Data:        []byte("log data"),
		Address:     address,
		Topics:      []common.Hash{topic},
		Index:       0,
	}

	manager.BroadcastLog(eventKey, logEvent)

	receivedLog1 := <-ch1
	receivedLog2 := <-ch2

	assert.Equal(t, logEvent, receivedLog1, "Subscriber 1 should receive the log")
	assert.Equal(t, logEvent, receivedLog2, "Subscriber 2 should receive the log")
}

func TestSubscriptionManager_Unsubscribe(t *testing.T) {
	manager := setupSubscriptionManager(t)

	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")

	// Subscribe to an event
	ch, err := manager.Subscribe(address, topic)
	require.NoError(t, err)
	assert.NotNil(t, ch)

	// Unsubscribe existing channel
	err = manager.Unsubscribe(address, topic, ch)
	assert.NoError(t, err)

	// Try unsubscribing again (should fail)
	err = manager.Unsubscribe(address, topic, ch)
	assert.Error(t, err)

	// Unsubscribe non-existent event key
	otherCh := make(chan api.Log)
	err = manager.Unsubscribe(address, topic, otherCh)
	assert.Error(t, err)

	// Check registry state
	manager.registryMutex.RLock()
	defer manager.registryMutex.RUnlock()
	assert.Len(t, manager.registry, 0, "Registry should be empty after unsubscribing")
}

func TestSubscriptionManager_UnsubscribeSelective(t *testing.T) {
	manager := setupSubscriptionManager(t)

	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	eventKey := internal.EventKey{Address: address, Topic: topic}

	ch1, err := manager.Subscribe(address, topic)
	require.NoError(t, err)

	ch2, err := manager.Subscribe(address, topic)
	require.NoError(t, err)

	// Unsubscribe one consumer and ensure the other remains
	err = manager.Unsubscribe(address, topic, ch1)
	require.NoError(t, err)

	// Check registry state
	manager.registryMutex.RLock()
	subscribers := manager.registry[eventKey]
	manager.registryMutex.RUnlock()

	assert.Len(t, subscribers, 1, "There should be one remaining channel after unsubscription")
	assert.Equal(t, ch2, subscribers[0], "The remaining channel should be the second subscriber")

	// Unsubscribe the last consumer and ensure the registry is cleaned up
	err = manager.Unsubscribe(address, topic, ch2)
	require.NoError(t, err)

	// Check registry state
	manager.registryMutex.RLock()
	_, exists := manager.registry[eventKey]
	manager.registryMutex.RUnlock()

	assert.False(t, exists, "The EventKey should no longer exist in the registry after the last subscriber unsubscribes")
}

func TestSubscriptionManager_BroadcastLog(t *testing.T) {
	manager := setupSubscriptionManager(t)

	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	eventKey := internal.EventKey{Address: address, Topic: topic}

	// Subscribe to an event
	ch, err := manager.Subscribe(address, topic)
	require.NoError(t, err)
	assert.NotNil(t, ch)

	logEvent := api.Log{
		BlockNumber: 1,
		TxHash:      common.HexToHash("0x1234"),
		Data:        []byte("log data"),
		Address:     address,
		Topics:      []common.Hash{topic},
		Index:       0,
	}

	// Broadcast log event
	manager.BroadcastLog(eventKey, logEvent)

	// Verify the channel received the event
	receivedLog := <-ch
	assert.Equal(t, logEvent, receivedLog, "Subscriber should receive the broadcasted log")
}

func TestSubscriptionManager_BroadcastToAllSubscribers(t *testing.T) {
	manager := setupSubscriptionManager(t)

	address1 := common.HexToAddress("0x9999567890abcdef1234567890abcdef12345678")
	topic1 := common.HexToHash("0xaaadefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	eventKey1 := internal.EventKey{Address: address1, Topic: topic1}

	address2 := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic2 := common.HexToHash("0xaaadefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	eventKey2 := internal.EventKey{Address: address2, Topic: topic2}

	ch1, err := manager.Subscribe(address1, topic1)
	require.NoError(t, err)

	ch2, err := manager.Subscribe(address2, topic2)
	require.NoError(t, err)

	ch3, err := manager.Subscribe(address1, topic1)
	require.NoError(t, err)

	// Broadcast a log and ensure all channels receive it
	logEvent1 := api.Log{
		BlockNumber: 2,
		TxHash:      common.HexToHash("0x5678"),
		Data:        []byte("another log data"),
		Address:     address1,
		Topics:      []common.Hash{topic1},
		Index:       0,
	}

	logEvent2 := api.Log{
		BlockNumber: 3,
		TxHash:      common.HexToHash("0x2345"),
		Data:        []byte("another log data 2"),
		Address:     address2,
		Topics:      []common.Hash{topic2},
		Index:       0,
	}

	manager.BroadcastLog(eventKey1, logEvent1)
	manager.BroadcastLog(eventKey2, logEvent2)

	receivedLog1 := <-ch1
	receivedLog2 := <-ch2
	receivedLog3 := <-ch3

	assert.Equal(t, logEvent1, receivedLog1, "Subscriber 1 should receive the log")
	assert.Equal(t, logEvent2, receivedLog2, "Subscriber 2 should receive the log")
	assert.Equal(t, logEvent1, receivedLog3, "Subscriber 3 should receive the log")
}

func TestSubscriptionManager_GetAddressesAndTopics(t *testing.T) {
	manager := setupSubscriptionManager(t)

	address1 := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic1 := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")

	address2 := common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdef")
	topic2 := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	_, err := manager.Subscribe(address1, topic1)
	require.NoError(t, err)

	_, err = manager.Subscribe(address2, topic2)
	require.NoError(t, err)

	// Fetch addresses and topics
	result := manager.GetAddressesAndTopics()

	// Verify addresses and topics
	assert.Contains(t, result, address1, "Address1 should be in map")
	assert.Contains(t, result, address2, "Address2 should be in map")
	assert.ElementsMatch(t, result[address1], []common.Hash{topic1}, "Address1 should contain topic1")
	assert.ElementsMatch(t, result[address2], []common.Hash{topic2}, "Address2 should contain topic2")
	assert.NotContains(t, result[address1], topic2, "Topic2 should not be in address1")
	assert.NotContains(t, result[address2], topic1, "Topic1 should not be in address2")
}

func TestSubscriptionManager_Cache(t *testing.T) {
	manager := setupSubscriptionManager(t)
	assert.False(t, manager.cacheInitialized, "Cache should not be initialized when Subscription Manager is initialized.")

	address1 := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic1 := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")

	address2 := common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdef")
	topic2 := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	// Initialize expected structure
	expectedCache := make(map[common.Address][]common.Hash)

	// Step 1: Subscribe to an event
	_, err := manager.Subscribe(address1, topic1)
	require.NoError(t, err)
	assert.False(t, manager.cacheInitialized, "Cache should not be initialized after Subscribe.")

	// Update expected structure
	expectedCache[address1] = []common.Hash{topic1}

	// Verify cache matches expected structure
	cache := manager.GetAddressesAndTopics()
	assert.True(t, manager.cacheInitialized, "Cache should be initialized after GetAddressesAndTopics() is called.")
	assert.Equal(t, expectedCache, cache, "Cache should match the expected structure.")

	// Step 2: Add another subscription
	ch, err := manager.Subscribe(address2, topic2)
	require.NoError(t, err)
	assert.False(t, manager.cacheInitialized, "Cache should be invalidated after Subscribe.")

	// Update expected structure
	expectedCache[address2] = []common.Hash{topic2}

	// Verify cache matches updated structure
	cache = manager.GetAddressesAndTopics()
	assert.True(t, manager.cacheInitialized, "Cache should be reinitialized after GetAddressesAndTopics() is called.")
	assert.Equal(t, expectedCache, cache, "Cache should match the updated structure.")

	// Step 3: Add a duplicate subscription for address1/topic1
	ch2, err := manager.Subscribe(address1, topic1)
	require.NoError(t, err)
	assert.False(t, manager.cacheInitialized, "Cache should be invalidated after Subscribe.")

	// No change to expected structure since it's a duplicate subscription
	cache = manager.GetAddressesAndTopics()
	assert.True(t, manager.cacheInitialized, "Cache should be reinitialized after GetAddressesAndTopics() is called.")
	assert.Equal(t, expectedCache, cache, "Cache should remain unchanged for duplicate subscriptions.")

	// Step 4: Unsubscribe from address2/topic2
	err = manager.Unsubscribe(address2, topic2, ch)
	require.NoError(t, err)
	assert.False(t, manager.cacheInitialized, "Cache should be invalidated after Unsubscribe.")

	// Update expected structure
	delete(expectedCache, address2)

	// Verify cache matches updated structure
	cache = manager.GetAddressesAndTopics()
	assert.True(t, manager.cacheInitialized, "Cache should be reinitialized after GetAddressesAndTopics() is called.")
	assert.Equal(t, expectedCache, cache, "Cache should match the updated structure after unsubscription.")

	// Step 5: Unsubscribe from non-existent subscription
	err = manager.Unsubscribe(address2, topic2, ch)
	assert.Error(t, err, "Unsubscribing a non-existent subscription should return an error.")

	// Ensure expected structure remains unchanged
	cache = manager.GetAddressesAndTopics()
	assert.True(t, manager.cacheInitialized, "Cache should remain initialized after an invalid unsubscribe attempt.")
	assert.Equal(t, expectedCache, cache, "Cache should remain unchanged for invalid unsubscribe attempts.")
	assert.Len(t, manager.registry[internal.EventKey{Address: address1, Topic: topic1}], 2, "EventKey should have two subscribers")

	// Step 6: Unsubscribe from address1, topic1, ch2
	err = manager.Unsubscribe(address1, topic1, ch2)
	require.NoError(t, err)
	assert.False(t, manager.cacheInitialized, "Cache should be invalidated after Unsubscribe.")

	cache = manager.GetAddressesAndTopics()
	assert.True(t, manager.cacheInitialized, "Cache should be reinitialized after GetAddressesAndTopics() is called.")
	assert.Equal(t, expectedCache, cache, "Cache should remain unchanged for duplicate subscriptions.")
	assert.Len(t, manager.registry[internal.EventKey{Address: address1, Topic: topic1}], 1, "EventKey should have two subscribers")
}

func TestSubscriptionManager_Close(t *testing.T) {
	manager := setupSubscriptionManager(t)

	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	topic := common.HexToHash("0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")

	// Subscribe to an event
	ch, err := manager.Subscribe(address, topic)
	require.NoError(t, err)
	assert.NotNil(t, ch)

	// Close the SubscriptionManager
	manager.Close()

	// Verify channel is closed
	_, open := <-ch
	assert.False(t, open, "Channel should be closed after Close()")

	// Verify registry is empty
	manager.registryMutex.RLock()
	defer manager.registryMutex.RUnlock()
	assert.Len(t, manager.registry, 0, "Registry should be empty after Close()")
}
