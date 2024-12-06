// File: internal/mock_blockchain_client.go
package internal

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockBlockchainClient implements the internal.BlockchainClient interface for testing.
type MockBlockchainClient struct {
	mock.Mock
}

func (m *MockBlockchainClient) BlockNumber(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockBlockchainClient) FilterLogs(ctx context.Context, query FilterQuery) ([]Log, error) {
	args := m.Called(ctx, query)
	return args.Get(0).([]Log), args.Error(1)
}

// Ensure MockBlockchainClient implements BlockchainClient interface
var _ BlockchainClient = (*MockBlockchainClient)(nil)
