package client

import (
	"github.com/rs/zerolog/log"
	"strconv"
	"time"
)

// RemoteAnvilMiner is a remote miner for Anvil node
// Allows to control blocks emission more precisely to mimic real networks workload
type RemoteAnvilMiner struct {
	Client            *AnvilClient
	interval          time.Duration
	batchSendInterval time.Duration
	batchCapacity     int64
	stop              chan struct{}
}

// NewRemoteAnvilMiner creates a new remote miner client
func NewRemoteAnvilMiner(url string) *RemoteAnvilMiner {
	return &RemoteAnvilMiner{
		Client: NewAnvilClient(url),
		stop:   make(chan struct{}),
	}
}

// MinePeriodically mines blocks with a specified interval
// should be used when Anvil mining is off
func (m *RemoteAnvilMiner) MinePeriodically(interval time.Duration) {
	m.interval = interval
	go func() {
		for {
			select {
			case <-m.stop:
				log.Info().Msg("anvil miner exiting")
				return
			default:
				if err := m.Client.Mine(nil); err != nil {
					log.Err(err).Send()
				}
			}
			time.Sleep(m.interval)
		}
	}()
}

// Stop stops the miner
func (m *RemoteAnvilMiner) Stop() {
	m.stop <- struct{}{}
}

// MineBatch checks the pending transactions in the pool, if threshold is reached mines the block and repeat the process
func (m *RemoteAnvilMiner) MineBatch(capacity int64, checkInterval time.Duration, sendInterval time.Duration) {
	m.interval = checkInterval
	m.batchCapacity = capacity
	m.batchSendInterval = sendInterval
	ticker := time.NewTicker(m.batchSendInterval)
	go func() {
		for {
			resp, err := m.Client.TxPoolStatus(nil)
			if err != nil {
				log.Err(err).Send()
			}
			pendingTx, err := strconv.ParseInt(resp.Result.Pending[2:], 16, 64)
			if err != nil {
				log.Err(err).Msg("failed to convert pending tx from hex to dec")
			}
			log.Info().Int64("Pending", pendingTx).Msg("Batch has pending transactions")
			if pendingTx >= m.batchCapacity {
				if err := m.Client.Mine(nil); err != nil {
					log.Err(err).Send()
				}
				log.Info().Int64("Transactions", pendingTx).Msg("Block mined")
			}
			select {
			case <-m.stop:
				log.Info().Msg("anvil miner exiting")
				ticker.Stop()
				return
			case <-ticker.C:
				if err := m.Client.Mine(nil); err != nil {
					log.Err(err).Send()
				}
				log.Info().Int64("Transactions", pendingTx).Msg("Block mined by timeout")
			default:
			}
			time.Sleep(m.interval)
		}
	}()
}
