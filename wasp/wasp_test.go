package wasp

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestSmokeConcurrentGenerators(t *testing.T) {
	t.Parallel()
	gens := make([]*Generator, 0)
	for i := 0; i < 2; i++ {
		gen, err := NewGenerator(&Config{
			T:        t,
			LoadType: RPS,
			Schedule: Plain(1, 1*time.Second),
			Gun: NewMockGun(&MockGunConfig{
				CallSleep: 50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		gen.Run(false)
		gens = append(gens, gen)
	}
	for _, gen := range gens {
		_, failed := gen.Wait()
		require.Equal(t, false, failed)
		stats := gen.Stats()
		require.Equal(t, int64(1), stats.CurrentRPS.Load())
		// we do not check exact RPS, because ratelimit.Limiter implementation
		// compensate RPS only after several requests
		// see example_test.go for precision tests of long runs
		// https://github.com/uber-go/ratelimit/blob/a12885fa6127db0aa3c29d33fc8ddeeb1fa1530c/limiter_atomic.go#L54
		require.GreaterOrEqual(t, stats.Success.Load(), int64(2))

		okData, okResponses, failResponses := convertResponsesData(gen)
		require.Contains(t, okData, "successCallData")
		require.GreaterOrEqual(t, len(okData), 2)
		require.GreaterOrEqual(t, okResponses[0].Duration, 50*time.Millisecond)
		require.Equal(t, okResponses[0].Data.(string), "successCallData")
		require.Equal(t, okResponses[1].Data.(string), "successCallData")
		require.GreaterOrEqual(t, len(okResponses), 2)
		require.Empty(t, failResponses)
		require.Empty(t, gen.Errors())
	}
}

func TestSmokePositiveOneRequest(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          RPS,
		StatsPollInterval: 1 * time.Second,
		Schedule:          Plain(1, 1*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 50 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	stats := gen.Stats()
	require.Equal(t, stats.CurrentRPS.Load(), int64(1))
	require.Equal(t, stats.CurrentVUs.Load(), int64(0))
	require.GreaterOrEqual(t, stats.Success.Load(), int64(2))
	require.Equal(t, stats.CallTimeout.Load(), int64(0))
	require.Equal(t, stats.Failed.Load(), int64(0))
	require.Equal(t, stats.Duration, gen.Cfg.duration.Nanoseconds())

	okData, okResponses, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, len(okResponses), 2)
	require.GreaterOrEqual(t, len(okData), 2)
	require.Equal(t, okData[0], "successCallData")
	require.Equal(t, okData[1], "successCallData")
	require.GreaterOrEqual(t, okResponses[0].Duration, 50*time.Millisecond)
	require.GreaterOrEqual(t, okResponses[1].Duration, 50*time.Millisecond)
	require.Equal(t, okResponses[0].Data.(string), "successCallData")
	require.Equal(t, okResponses[1].Data.(string), "successCallData")
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestSmokePositiveCustomRateLimitUnit(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          RPS,
		StatsPollInterval: 2 * time.Second,
		Schedule:          Plain(1, 5*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 10 * time.Millisecond,
		}),
		RateLimitUnitDuration: 2 * time.Second,
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	stats := gen.Stats()
	// at least 4 requests, 1 request per 2 seconds
	require.GreaterOrEqual(t, stats.Success.Load(), int64(4))
	require.Equal(t, stats.CurrentRPS.Load(), int64(1))

	okData, okResponses, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, okResponses[0].Duration, 10*time.Millisecond)
	require.GreaterOrEqual(t, okResponses[3].Duration, 10*time.Millisecond)
	require.GreaterOrEqual(t, len(okData), 4)
	require.Equal(t, okData[0], "successCallData")
	require.Equal(t, okData[3], "successCallData")
	require.GreaterOrEqual(t, len(okResponses), 4)
	require.Equal(t, okResponses[0].Data.(string), "successCallData")
	require.Equal(t, okResponses[3].Data.(string), "successCallData")
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestSmokeGenCanBeStoppedMultipleTimes(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          RPS,
		StatsPollInterval: 1 * time.Second,
		Schedule:          Plain(1, 1*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			InternalStop: true,
			CallSleep:    50 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, _ = gen.Run(false)
	time.Sleep(60 * time.Millisecond)
	var failed bool
	for i := 0; i < 10; i++ {
		_, failed = gen.Stop()
	}
	require.Equal(t, true, failed)
	stats := gen.Stats()
	require.GreaterOrEqual(t, stats.Success.Load(), int64(0))
	require.Equal(t, stats.RunStopped.Load(), true)
	require.Equal(t, stats.RunFailed.Load(), true)
	require.Equal(t, stats.CurrentRPS.Load(), int64(1))
}

func TestSmokeFailedOneRequest(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          RPS,
		StatsPollInterval: 1 * time.Second,
		Schedule:          Plain(1, 1*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			FailRatio: 100,
			CallSleep: 50 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, _ = gen.Run(false)
	time.Sleep(40 * time.Millisecond)
	_, failed := gen.Stop()
	require.Equal(t, true, failed)
	stats := gen.Stats()
	require.GreaterOrEqual(t, stats.Failed.Load(), int64(1))
	require.Equal(t, stats.RunFailed.Load(), true)
	require.Equal(t, stats.CurrentRPS.Load(), int64(1))
	require.Equal(t, stats.Duration, gen.Cfg.duration.Nanoseconds())

	okData, _, failResponses := convertResponsesData(gen)
	require.Empty(t, okData)
	require.GreaterOrEqual(t, failResponses[0].Duration, 50*time.Millisecond)
	require.Equal(t, failResponses[0].Data.(string), "failedCallData")
	require.Equal(t, failResponses[0].Error, "error")
	errs := gen.Errors()
	require.Equal(t, errs[0], "error")
	require.GreaterOrEqual(t, len(errs), 1)
}

func TestSmokeGenCallTimeout(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:           t,
		LoadType:    RPS,
		Schedule:    Plain(1, 1*time.Second),
		CallTimeout: 400 * time.Millisecond,
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 500 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	gen.Run(false)
	time.Sleep(550 * time.Millisecond)
	_, failed := gen.Stop()
	require.Equal(t, true, failed)
	stats := gen.Stats()
	require.GreaterOrEqual(t, stats.Success.Load(), int64(0))
	require.GreaterOrEqual(t, stats.CallTimeout.Load(), int64(1))
	require.Equal(t, stats.CurrentRPS.Load(), int64(1))

	okData, _, failResponses := convertResponsesData(gen)
	require.Empty(t, okData)
	require.Equal(t, failResponses[0].Data, nil)
	require.Equal(t, failResponses[0].Error, ErrCallTimeout.Error())
	require.Equal(t, gen.Errors()[0], ErrCallTimeout.Error())
}

func TestSmokeVUCallTimeout(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          VU,
		StatsPollInterval: 1 * time.Second,
		Schedule:          Plain(1, 1000*time.Millisecond),
		CallTimeout:       900 * time.Millisecond,
		VU: NewMockVU(&MockVirtualUserConfig{
			CallSleep: 905 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, true, failed)
	stats := gen.Stats()
	require.GreaterOrEqual(t, stats.Success.Load(), int64(0))
	require.GreaterOrEqual(t, stats.Failed.Load(), int64(1))
	require.GreaterOrEqual(t, stats.CallTimeout.Load(), int64(1))
	require.Equal(t, stats.CurrentVUs.Load(), int64(1))

	// in case of VU call timeout we mark them as timed out,
	// proceeding to the next call and store no data
	okData, _, failResponses := convertResponsesData(gen)
	require.Empty(t, okData)
	require.Equal(t, failResponses[0].Data, nil)
	require.Equal(t, ErrCallTimeout.Error(), failResponses[0].Error)
	require.Contains(t, gen.Errors(), ErrCallTimeout.Error())
	require.GreaterOrEqual(t, len(gen.Errors()), 1)
}

func TestSmokeVUSetupTeardownNegativeCases(t *testing.T) {
	t.Parallel()
	t.Run("setup failure counts as CallResult error", func(t *testing.T) {
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          VU,
			StatsPollInterval: 1 * time.Second,
			Schedule:          Plain(1, 1000*time.Millisecond),
			SetupTimeout:      900 * time.Millisecond,
			VU: NewMockVU(&MockVirtualUserConfig{
				SetupFailure: true,
				CallSleep:    50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, failed := gen.Run(true)
		require.Equal(t, true, failed)
		stats := gen.Stats()
		require.Equal(t, stats.Success.Load(), int64(0))
		require.Equal(t, stats.Failed.Load(), int64(1))
		require.Equal(t, stats.CallTimeout.Load(), int64(0))
		require.Equal(t, stats.CurrentVUs.Load(), int64(1))
	})
	t.Run("teardown error counts as CallResult error", func(t *testing.T) {
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          VU,
			StatsPollInterval: 1 * time.Second,
			Schedule:          Steps(10, -1, 10, 1000*time.Millisecond),
			VU: NewMockVU(&MockVirtualUserConfig{
				TeardownFailure: true,
				CallSleep:       50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, failed := gen.Run(true)
		require.Equal(t, true, failed)
		stats := gen.Stats()
		require.GreaterOrEqual(t, stats.Success.Load(), int64(50))
		require.GreaterOrEqual(t, stats.Failed.Load(), int64(8))
		require.GreaterOrEqual(t, stats.CallTimeout.Load(), int64(0))
		require.Equal(t, stats.CurrentVUs.Load(), int64(1))
	})
	t.Run("setup timeout counts as CallResult error", func(t *testing.T) {
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          VU,
			StatsPollInterval: 1 * time.Second,
			Schedule:          Plain(1, 1000*time.Millisecond),
			SetupTimeout:      900 * time.Millisecond,
			VU: NewMockVU(&MockVirtualUserConfig{
				SetupSleep: 950 * time.Millisecond,
				CallSleep:  50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, failed := gen.Run(true)
		require.Equal(t, true, failed)
		stats := gen.Stats()
		require.Equal(t, stats.Success.Load(), int64(0))
		require.Equal(t, stats.Failed.Load(), int64(1))
		require.Equal(t, stats.CallTimeout.Load(), int64(1))
		require.Equal(t, stats.CurrentVUs.Load(), int64(1))
	})
	t.Run("teardown timeout counts as CallResult error", func(t *testing.T) {
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          VU,
			StatsPollInterval: 1 * time.Second,
			Schedule:          Steps(10, -1, 10, 1000*time.Millisecond),
			TeardownTimeout:   100 * time.Millisecond,
			VU: NewMockVU(&MockVirtualUserConfig{
				TeardownSleep: 950 * time.Millisecond,
				CallSleep:     50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, failed := gen.Run(true)
		require.Equal(t, true, failed)
		stats := gen.Stats()
		require.GreaterOrEqual(t, stats.Success.Load(), int64(50))
		require.GreaterOrEqual(t, stats.Failed.Load(), int64(8))
		require.GreaterOrEqual(t, stats.CallTimeout.Load(), int64(8))
		require.Equal(t, stats.CurrentVUs.Load(), int64(1))
	})
}

func TestSmokeGenCallTimeoutWait(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:           t,
		LoadType:    RPS,
		Schedule:    Plain(1, 1*time.Second),
		CallTimeout: 50 * time.Millisecond,
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 55 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, true, failed)
	stats := gen.Stats()
	require.Equal(t, int64(1), stats.CurrentRPS.Load())
	require.GreaterOrEqual(t, stats.CallTimeout.Load(), int64(1))
	require.Equal(t, true, stats.RunFailed.Load())

	okData, _, failResponses := convertResponsesData(gen)
	require.Empty(t, okData)
	require.Equal(t, failResponses[0].Data, nil)
	require.Equal(t, failResponses[0].Error, ErrCallTimeout.Error())
	require.Contains(t, gen.Errors(), ErrCallTimeout.Error())
	require.GreaterOrEqual(t, len(gen.Errors()), 1)
}

func TestSmokeCancelledByDeadlineWait(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		StatsPollInterval: 1 * time.Second,
		LoadType:          RPS,
		Schedule:          Plain(1, 40*time.Millisecond),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 50 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	gen.Run(false)
	_, failed := gen.Wait()

	require.Equal(t, false, failed)
	stats := gen.Stats()
	require.GreaterOrEqual(t, stats.Success.Load(), int64(2))
	require.Equal(t, stats.CurrentRPS.Load(), int64(1))
	require.Equal(t, stats.Duration, gen.Cfg.duration.Nanoseconds())
	require.Equal(t, stats.CurrentTimeUnit, gen.Cfg.RateLimitUnitDuration.Nanoseconds())

	// in case of gen.Stop() if we don't have test duration or if gen.Wait() and we have a deadline
	// we are waiting for all requests, so result in that case must be successful
	okData, _, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, len(okData), 2)
	require.Equal(t, okData[0], "successCallData")
	require.Equal(t, okData[1], "successCallData")
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestStopOnFirstFailure(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          RPS,
		FailOnErr:         true,
		CallTimeout:       600 * time.Millisecond,
		StatsPollInterval: 1 * time.Second,
		Schedule:          Plain(1, 10*time.Minute),
		Gun: NewMockGun(&MockGunConfig{
			FailRatio: 100,
			CallSleep: 500 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, true, failed)
}

func TestSmokeStaticRPSSchedulePrecision(t *testing.T) {
	gen, err := NewGenerator(&Config{
		T:        t,
		LoadType: RPS,
		Schedule: Plain(1000, 1*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 50 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	require.GreaterOrEqual(t, gen.Stats().Success.Load(), int64(980))
	require.LessOrEqual(t, gen.Stats().Success.Load(), int64(1010))
	require.Equal(t, gen.Stats().Failed.Load(), int64(0))
	require.Equal(t, gen.Stats().CallTimeout.Load(), int64(0))

	okData, _, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, len(okData), 980)
	require.LessOrEqual(t, len(okData), 1010)
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestSmokeCustomUnitPrecision(t *testing.T) {
	gen, err := NewGenerator(&Config{
		T:                     t,
		LoadType:              RPS,
		RateLimitUnitDuration: 2 * time.Second,
		Schedule:              Plain(1000, 10*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 10 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	stats := gen.Stats()
	require.GreaterOrEqual(t, stats.Success.Load(), int64(4970))
	require.LessOrEqual(t, stats.Success.Load(), int64(5010))
	require.Equal(t, stats.Failed.Load(), int64(0))
	require.Equal(t, stats.CallTimeout.Load(), int64(0))
	require.Equal(t, stats.CurrentTimeUnit, gen.Cfg.RateLimitUnitDuration.Nanoseconds())

	okData, _, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, len(okData), 4970)
	require.LessOrEqual(t, len(okData), 5010)
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestSmokeStaticRPSScheduleIsNotBlocking(t *testing.T) {
	gen, err := NewGenerator(&Config{
		T:        t,
		LoadType: RPS,
		Schedule: Plain(1000, 1*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			// call time must not affect the load scheduleSegments
			CallSleep: 1 * time.Second,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	require.GreaterOrEqual(t, gen.Stats().Success.Load(), int64(980))
	require.LessOrEqual(t, gen.Stats().Success.Load(), int64(1010))
	require.Equal(t, gen.Stats().Failed.Load(), int64(0))
	require.Equal(t, gen.Stats().CallTimeout.Load(), int64(0))

	okData, _, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, len(okData), 980)
	require.LessOrEqual(t, len(okData), 1010)
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestSmokeLoadScheduleSegmentRPSIncrease(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		StatsPollInterval: 1 * time.Second,
		LoadType:          RPS,
		Schedule: Combine(
			Plain(1, 5*time.Second),
			Plain(2, 5*time.Second),
		),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 10 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	require.GreaterOrEqual(t, gen.Stats().Success.Load(), int64(16))
}

func TestSmokeLoadScheduleSegmentRPSDecrease(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		StatsPollInterval: 1 * time.Second,
		LoadType:          RPS,
		Schedule: Combine(
			Plain(2, 5*time.Second),
			Plain(1, 5*time.Second),
		),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 10 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	require.GreaterOrEqual(t, gen.Stats().Success.Load(), int64(17))
}

func TestSmokeValidation(t *testing.T) {
	t.Parallel()
	t.Run("can start with 0 RPS", func(t *testing.T) {
		t.Parallel()
		_, err := NewGenerator(&Config{
			T:                 t,
			StatsPollInterval: 1 * time.Second,
			LoadType:          RPS,
			Schedule: []*Segment{
				{
					From:     0,
					Duration: 1 * time.Second,
				},
			},
			Gun: NewMockGun(&MockGunConfig{
				CallSleep: 10 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
	})
	t.Run("can't start with invalid segment definition", func(t *testing.T) {
		t.Parallel()
		_, err := NewGenerator(&Config{
			T:                 t,
			StatsPollInterval: 1 * time.Second,
			LoadType:          RPS,
			Schedule: []*Segment{
				{
					From: 1,
				},
			},
			Gun: NewMockGun(&MockGunConfig{
				CallSleep: 10 * time.Millisecond,
			}),
		})
		require.Equal(t, ErrInvalidSegmentDuration, err)
		_, err = NewGenerator(&Config{
			T:                 t,
			StatsPollInterval: 1 * time.Second,
			LoadType:          RPS,
			Schedule: []*Segment{
				{
					From: 1,
				},
			},
			Gun: NewMockGun(&MockGunConfig{
				CallSleep: 10 * time.Millisecond,
			}),
		})
		require.Equal(t, ErrInvalidSegmentDuration, err)
	})
	t.Run("can't start with nil cfg", func(t *testing.T) {
		t.Parallel()
		_, err := NewGenerator(nil)
		require.Equal(t, ErrNoCfg, err)
	})
	t.Run("can't start without gun/vu implementation", func(t *testing.T) {
		t.Parallel()
		_, err := NewGenerator(&Config{
			T:        t,
			LoadType: RPS,
			Schedule: []*Segment{
				{
					From: 1,
				},
			},
			Gun: nil,
		})
		require.Equal(t, ErrNoImpl, err)
	})
	t.Run("can't start with invalid workload type", func(t *testing.T) {
		t.Parallel()
		_, err := NewGenerator(&Config{
			T:        t,
			LoadType: "arbitrary_load_type",
			Schedule: []*Segment{
				{
					From: 1,
				},
			},
			Gun: NewMockGun(&MockGunConfig{
				CallSleep: 10 * time.Millisecond,
			}),
		})
		require.Equal(t, ErrInvalidScheduleType, err)
	})
	t.Run("can't start with invalid labels", func(t *testing.T) {
		t.Parallel()
		_, err := NewGenerator(&Config{
			T:        t,
			LoadType: RPS,
			Schedule: Plain(1, 1*time.Second),
			Gun:      NewMockGun(&MockGunConfig{}),
			Labels: map[string]string{
				"\\.[]{}()<>*+-=!?^$|": "\\.[]{}()<>*+-=!?^$|",
			},
		})
		require.Equal(t, ErrInvalidLabels, err)
	})
}

func TestSmokeVUsIncrease(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          VU,
		StatsPollInterval: 1 * time.Second,
		Schedule: Combine(
			Plain(1, 5*time.Second),
			Plain(2, 5*time.Second),
		),
		VU: NewMockVU(&MockVirtualUserConfig{
			CallSleep: 100 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	stats := gen.Stats()
	require.Equal(t, false, failed)
	require.Equal(t, int64(2), stats.CurrentVUs.Load())

	okData, okResponses, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, okResponses[0].Duration, 50*time.Millisecond)
	require.GreaterOrEqual(t, len(okResponses), 147)
	require.GreaterOrEqual(t, len(okData), 147)
	require.Equal(t, okResponses[0].Data.(string), "successCallData")
	require.Equal(t, okResponses[147].Data.(string), "successCallData")
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestSmokeVUsDecrease(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          VU,
		StatsPollInterval: 1 * time.Second,
		Schedule: Combine(
			Plain(2, 5*time.Second),
			Plain(1, 5*time.Second),
		),
		VU: NewMockVU(&MockVirtualUserConfig{
			CallSleep: 100 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	stats := gen.Stats()
	require.Equal(t, false, failed)
	require.Equal(t, int64(1), stats.CurrentVUs.Load())

	okData, okResponses, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, okResponses[0].Duration, 50*time.Millisecond)
	require.GreaterOrEqual(t, len(okResponses), 147)
	require.GreaterOrEqual(t, len(okData), 147)
	require.Equal(t, okResponses[0].Data.(string), "successCallData")
	require.Equal(t, okResponses[147].Data.(string), "successCallData")
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestSmokeVUsSetupTeardown(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		StatsPollInterval: 1 * time.Second,
		LoadType:          VU,
		Schedule: Combine(
			Plain(1, 10*time.Second),
			Plain(10, 10*time.Second),
		),
		VU: NewMockVU(&MockVirtualUserConfig{
			CallSleep: 100 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	require.GreaterOrEqual(t, gen.Stats().Success.Load(), int64(1080))
}

func TestSamplingSuccessfulResults(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          RPS,
		SamplerConfig:     &SamplerConfig{SuccessfulCallResultRecordRatio: 50},
		StatsPollInterval: 1 * time.Second,
		Schedule:          Plain(100, 1*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 50 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	// roughly 50% of samples recorded
	stats := gen.Stats()
	require.GreaterOrEqual(t, stats.SamplesRecorded.Load(), int64(35))
	require.LessOrEqual(t, stats.SamplesRecorded.Load(), int64(65))
	require.GreaterOrEqual(t, stats.SamplesSkipped.Load(), int64(35))
	require.LessOrEqual(t, stats.SamplesSkipped.Load(), int64(65))
	_, okResponses, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, len(okResponses), 35)
	require.LessOrEqual(t, len(okResponses), 65)
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestProfiles(t *testing.T) {
	t.Parallel()
	t.Run("fail fast on setup if generator config is invalid", func(t *testing.T) {
		t.Parallel()
		_, err := NewProfile().
			Add(NewGenerator(&Config{
				T:        t,
				LoadType: RPS,
				GenName:  "A",
				Schedule: Plain(2, 5*time.Second),
				Gun: NewMockGun(&MockGunConfig{
					CallSleep: 50 * time.Millisecond,
				}),
			})).
			Add(NewGenerator(&Config{})).
			Run(true)
		require.Error(t, err)
	})
	t.Run("runs in parallel and have results", func(t *testing.T) {
		t.Parallel()
		p, err := NewProfile().
			Add(NewGenerator(&Config{
				T:        t,
				LoadType: RPS,
				GenName:  "A",
				Schedule: Plain(2, 5*time.Second),
				Gun: NewMockGun(&MockGunConfig{
					CallSleep: 50 * time.Millisecond,
				}),
			})).
			Add(NewGenerator(&Config{
				T:        t,
				LoadType: VU,
				GenName:  "B",
				Schedule: Plain(1, 5*time.Second),
				VU: NewMockVU(&MockVirtualUserConfig{
					CallSleep: 50 * time.Millisecond,
				}),
			})).
			Run(true)
		require.NoError(t, err)
		g1 := p.Generators[0]
		g1Stats := g1.Stats()
		require.Equal(t, int64(2), g1Stats.CurrentRPS.Load())

		okData, okResponses, failResponses := convertResponsesData(g1)
		require.GreaterOrEqual(t, okResponses[0].Duration, 50*time.Millisecond)
		require.Greater(t, len(okResponses), 10)
		require.Greater(t, len(okData), 10)
		require.Equal(t, okResponses[0].Data.(string), "successCallData")
		require.Equal(t, okResponses[10].Data.(string), "successCallData")
		require.Empty(t, failResponses)
		require.Empty(t, g1.Errors())

		g2 := p.Generators[1]
		g2Stats := g2.Stats()
		require.Equal(t, int64(1), g2Stats.CurrentVUs.Load())

		okData, okResponses, failResponses = convertResponsesData(g2)
		require.GreaterOrEqual(t, okResponses[0].Duration, 50*time.Millisecond)
		require.Greater(t, len(okResponses), 90)
		require.Greater(t, len(okData), 90)
		require.Equal(t, okResponses[0].Data.(string), "successCallData")
		require.Equal(t, okResponses[90].Data.(string), "successCallData")
		require.Empty(t, failResponses)
		require.Empty(t, g1.Errors())
	})

	t.Run("profile can be paused and resumed", func(t *testing.T) {
		t.Parallel()
		p, err := NewProfile().
			Add(NewGenerator(&Config{
				T:                 t,
				LoadType:          RPS,
				GenName:           "A",
				StatsPollInterval: 1 * time.Second,
				Schedule:          Plain(10, 9*time.Second),
				Gun: NewMockGun(&MockGunConfig{
					CallSleep: 50 * time.Millisecond,
				}),
			})).
			Add(NewGenerator(&Config{
				T:                 t,
				LoadType:          VU,
				GenName:           "B",
				StatsPollInterval: 1 * time.Second,
				Schedule:          Plain(1, 9*time.Second),
				VU: NewMockVU(&MockVirtualUserConfig{
					CallSleep: 50 * time.Millisecond,
				}),
			})).
			Run(false)
		time.Sleep(3 * time.Second)
		p.Pause()
		time.Sleep(3 * time.Second)
		p.Resume()
		time.Sleep(3 * time.Second)
		p.Wait()
		require.NoError(t, err)
		g1 := p.Generators[0]
		g1Stats := g1.Stats()
		_, okResponses, failResponses := convertResponsesData(g1)
		require.Equal(t, int64(10), g1Stats.CurrentRPS.Load())
		require.GreaterOrEqual(t, okResponses[0].Duration, 50*time.Millisecond)
		require.GreaterOrEqual(t, len(okResponses), 70)
		require.Empty(t, failResponses)
		require.Empty(t, g1.Errors())

		g2 := p.Generators[1]
		g2Stats := g2.Stats()
		_, okResponses, failResponses = convertResponsesData(g2)
		require.Equal(t, int64(1), g2Stats.CurrentVUs.Load())
		require.Greater(t, len(okResponses), 110)
		require.Empty(t, failResponses)
		require.Empty(t, g1.Errors())
	})
}

func TestSamplerStoresFailedResults(t *testing.T) {
	t.Parallel()
	t.Run("failed results are always stored - RPS", func(t *testing.T) {
		t.Parallel()
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          RPS,
			SamplerConfig:     &SamplerConfig{SuccessfulCallResultRecordRatio: 50},
			StatsPollInterval: 5 * time.Second,
			Schedule:          Plain(100, 4*time.Second),
			Gun: NewMockGun(&MockGunConfig{
				FailRatio: 100,
				CallSleep: 50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, failed := gen.Run(true)
		require.Equal(t, true, failed)
		stats := gen.Stats()
		require.GreaterOrEqual(t, stats.SamplesRecorded.Load(), int64(400))
		require.GreaterOrEqual(t, stats.SamplesSkipped.Load(), int64(0))
		_, _, failResponses := convertResponsesData(gen)
		require.GreaterOrEqual(t, len(failResponses), 400)
	})
	t.Run("failed results are always stored - VU", func(t *testing.T) {
		t.Parallel()
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          VU,
			SamplerConfig:     &SamplerConfig{SuccessfulCallResultRecordRatio: 50},
			StatsPollInterval: 5 * time.Second,
			Schedule:          Plain(10, 4*time.Second),
			VU: NewMockVU(&MockVirtualUserConfig{
				FailRatio: 100,
				CallSleep: 50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, failed := gen.Run(true)
		require.Equal(t, true, failed)
		stats := gen.Stats()
		require.GreaterOrEqual(t, stats.SamplesRecorded.Load(), int64(600))
		require.GreaterOrEqual(t, stats.SamplesSkipped.Load(), int64(0))
		_, _, failResponses := convertResponsesData(gen)
		require.GreaterOrEqual(t, len(failResponses), 600)
	})
	t.Run("timed out results are always stored - RPS", func(t *testing.T) {
		t.Parallel()
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          RPS,
			SamplerConfig:     &SamplerConfig{SuccessfulCallResultRecordRatio: 100},
			StatsPollInterval: 5 * time.Second,
			CallTimeout:       60 * time.Millisecond,
			Schedule:          Plain(100, 4*time.Second),
			Gun: NewMockGun(&MockGunConfig{
				TimeoutRatio: 100,
				CallSleep:    50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, failed := gen.Run(true)
		require.Equal(t, true, failed)
		stats := gen.Stats()
		require.GreaterOrEqual(t, stats.SamplesRecorded.Load(), int64(400))
		require.GreaterOrEqual(t, stats.SamplesSkipped.Load(), int64(0))
		_, _, failResponses := convertResponsesData(gen)
		require.GreaterOrEqual(t, len(failResponses), 400)
	})
	t.Run("timed out results are always stored - VU", func(t *testing.T) {
		t.Parallel()
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          VU,
			SamplerConfig:     &SamplerConfig{SuccessfulCallResultRecordRatio: 5},
			StatsPollInterval: 5 * time.Second,
			CallTimeout:       60 * time.Millisecond,
			Schedule:          Plain(10, 4*time.Second),
			VU: NewMockVU(&MockVirtualUserConfig{
				TimeoutRatio: 100,
				CallSleep:    50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, failed := gen.Run(true)
		require.Equal(t, true, failed)
		stats := gen.Stats()
		require.GreaterOrEqual(t, stats.SamplesRecorded.Load(), int64(400))
		require.GreaterOrEqual(t, stats.SamplesSkipped.Load(), int64(0))
		_, _, failResponses := convertResponsesData(gen)
		require.GreaterOrEqual(t, len(failResponses), 600)
	})
}

func TestSmokePauseResumeGenerator(t *testing.T) {
	t.Parallel()
	t.Run("can pause RPS generator", func(t *testing.T) {
		t.Parallel()
		gen, err := NewGenerator(&Config{
			T:                     t,
			LoadType:              RPS,
			RateLimitUnitDuration: 1 * time.Second,
			StatsPollInterval:     1 * time.Second,
			Schedule:              Plain(10, 9*time.Second),
			Gun: NewMockGun(&MockGunConfig{
				CallSleep: 50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, _ = gen.Run(false)
		time.Sleep(3 * time.Second)
		gen.Pause()
		time.Sleep(3 * time.Second)
		gen.Resume()
		time.Sleep(3 * time.Second)
		_, failed := gen.Wait()
		require.Equal(t, false, failed)
		stats := gen.Stats()
		_, okResponses, failResponses := convertResponsesData(gen)
		require.Equal(t, int64(10), stats.CurrentRPS.Load())
		require.GreaterOrEqual(t, len(okResponses), 60)
		require.Empty(t, failResponses)
		require.Empty(t, gen.Errors())
	})
	t.Run("can pause VU generator", func(t *testing.T) {
		t.Parallel()
		gen, err := NewGenerator(&Config{
			T:                 t,
			LoadType:          VU,
			StatsPollInterval: 1 * time.Second,
			Schedule:          Plain(1, 9*time.Second),
			VU: NewMockVU(&MockVirtualUserConfig{
				CallSleep: 50 * time.Millisecond,
			}),
		})
		require.NoError(t, err)
		_, _ = gen.Run(false)
		time.Sleep(3 * time.Second)
		gen.Pause()
		time.Sleep(3 * time.Second)
		gen.Resume()
		time.Sleep(3 * time.Second)
		_, failed := gen.Wait()
		require.Equal(t, false, failed)

		stats := gen.Stats()
		_, okResponses, failResponses := convertResponsesData(gen)
		require.Equal(t, int64(1), stats.CurrentVUs.Load())
		require.GreaterOrEqual(t, len(okResponses), 110)
		require.Empty(t, failResponses)
		require.Empty(t, gen.Errors())
	})
}

// regression

func TestSmokeNoDuplicateRequestsOnceOnStart(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		LoadType:          RPS,
		StatsPollInterval: 1 * time.Second,
		Schedule:          Plain(1, 1*time.Second),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 50 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(false)
	require.Equal(t, false, failed)
	time.Sleep(950 * time.Millisecond)
	_, _ = gen.Stop()
	stats := gen.Stats()
	require.Equal(t, stats.CurrentRPS.Load(), int64(1))
	require.Equal(t, stats.CurrentVUs.Load(), int64(0))
	require.GreaterOrEqual(t, stats.Success.Load(), int64(1))
	require.Equal(t, stats.CallTimeout.Load(), int64(0))
	require.Equal(t, stats.Failed.Load(), int64(0))
	require.Equal(t, stats.Duration, gen.Cfg.duration.Nanoseconds())

	okData, okResponses, failResponses := convertResponsesData(gen)
	require.GreaterOrEqual(t, len(okResponses), 1)
	require.GreaterOrEqual(t, len(okData), 1)
	require.Equal(t, okData[0], "successCallData")
	require.GreaterOrEqual(t, okResponses[0].Duration, 50*time.Millisecond)
	require.Equal(t, okResponses[0].Data.(string), "successCallData")
	require.Empty(t, failResponses)
	require.Empty(t, gen.Errors())
}

func TestZeroSchedule(t *testing.T) {
	t.Parallel()
	gen, err := NewGenerator(&Config{
		T:                 t,
		StatsPollInterval: 1 * time.Second,
		LoadType:          RPS,
		Schedule: Combine(
			Steps(0, 1, 10, 10*time.Second),
			Plain(0, 5*time.Second),
			Steps(10, -1, 10, 10*time.Second),
		),
		Gun: NewMockGun(&MockGunConfig{
			CallSleep: 10 * time.Millisecond,
		}),
	})
	require.NoError(t, err)
	_, failed := gen.Run(true)
	require.Equal(t, false, failed)
	require.GreaterOrEqual(t, gen.Stats().Success.Load(), int64(5))
}
