package wasp

import (
	"context"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.uber.org/ratelimit"
)

const (
	DefaultCallTimeout           = 1 * time.Minute
	DefaultSetupTimeout          = 1 * time.Minute
	DefaultTeardownTimeout       = 1 * time.Minute
	DefaultStatsPollInterval     = 5 * time.Second
	DefaultRateLimitUnitDuration = 1 * time.Second
	DefaultCallResultBufLen      = 50000
	DefaultGenName               = "Generator"
)

var (
	ErrNoCfg                  = errors.New("config is nil")
	ErrNoImpl                 = errors.New("either \"gun\" or \"vu\" implementation must provided")
	ErrNoSchedule             = errors.New("no schedule segments were provided")
	ErrInvalidScheduleType    = errors.New("schedule type must be either of wasp.RPS, wasp.VU, use package constants")
	ErrCallTimeout            = errors.New("generator request call timeout")
	ErrSetupTimeout           = errors.New("generator request setup timeout")
	ErrSetup                  = errors.New("generator request setup error")
	ErrTeardownTimeout        = errors.New("generator request teardown timeout")
	ErrTeardown               = errors.New("generator request teardown error")
	ErrStartFrom              = errors.New("from must be > 0")
	ErrInvalidSegmentDuration = errors.New("SegmentDuration must be defined")
	ErrNoGun                  = errors.New("rps load scheduleSegments selected but gun implementation is nil")
	ErrNoVU                   = errors.New("vu load scheduleSegments selected but vu implementation is nil")
	ErrInvalidLabels          = errors.New("invalid Loki labels, labels should be [a-z][A-Z][0-9] and _")
)

// Gun is basic interface for some synthetic load test implementation
// Call performs one request according to some RPS schedule
type Gun interface {
	Call(l *Generator) *Response
}

// VirtualUser is basic interface to run virtual users load
// you should use it if:
// - your protocol is stateful, ex.: ws, grpc
// - you'd like to have some VirtualUser modelling, perform sequential requests
type VirtualUser interface {
	Call(l *Generator)
	Clone(l *Generator) VirtualUser
	Setup(l *Generator) error
	Teardown(l *Generator) error
	Stop(l *Generator)
	StopChan() chan struct{}
}

// NewVUControl creates new base VU that allows us to control the schedule and bring VUs up and down
func NewVUControl() *VUControl {
	return &VUControl{stop: make(chan struct{}, 1)}
}

// VUControl is a base VU that allows us to control the schedule and bring VUs up and down
type VUControl struct {
	stop chan struct{}
}

// Stop stops virtual user execution
func (m *VUControl) Stop(_ *Generator) {
	m.stop <- struct{}{}
}

// StopChan returns stop chan
func (m *VUControl) StopChan() chan struct{} {
	return m.stop
}

// Response represents basic result info
type Response struct {
	Failed     bool          `json:"failed,omitempty"`
	Timeout    bool          `json:"timeout,omitempty"`
	StatusCode string        `json:"status_code,omitempty"`
	Path       string        `json:"path,omitempty"`
	Duration   time.Duration `json:"duration"`
	StartedAt  *time.Time    `json:"started_at,omitempty"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
	Group      string        `json:"group"`
	Data       interface{}   `json:"data,omitempty"`
	Error      string        `json:"error,omitempty"`
}

type ScheduleType string

const (
	RPS ScheduleType = "rps_schedule"
	VU  ScheduleType = "vu_schedule"
)

// Segment load test schedule segment
type Segment struct {
	From     int64
	Duration time.Duration
}

func (ls *Segment) Validate() error {
	if ls.From < 0 {
		return ErrStartFrom
	}
	if ls.Duration == 0 {
		return ErrInvalidSegmentDuration
	}
	return nil
}

// Config is for shared load test data and configuration
type Config struct {
	T                     *testing.T
	GenName               string
	LoadType              ScheduleType
	Labels                map[string]string
	LokiConfig            *LokiConfig
	Schedule              []*Segment
	RateLimitUnitDuration time.Duration
	CallResultBufLen      int
	StatsPollInterval     time.Duration
	CallTimeout           time.Duration
	SetupTimeout          time.Duration
	TeardownTimeout       time.Duration
	FailOnErr             bool
	Gun                   Gun
	VU                    VirtualUser
	Logger                zerolog.Logger
	SharedData            interface{}
	SamplerConfig         *SamplerConfig
	// calculated fields
	duration time.Duration
	// only available in cluster mode
	nodeID string
}

func (lgc *Config) Validate() error {
	if lgc.CallTimeout == 0 {
		lgc.CallTimeout = DefaultCallTimeout
	}
	if lgc.SetupTimeout == 0 {
		lgc.SetupTimeout = DefaultSetupTimeout
	}
	if lgc.TeardownTimeout == 0 {
		lgc.TeardownTimeout = DefaultTeardownTimeout
	}
	if lgc.StatsPollInterval == 0 {
		lgc.StatsPollInterval = DefaultStatsPollInterval
	}
	if lgc.CallResultBufLen == 0 {
		lgc.CallResultBufLen = DefaultCallResultBufLen
	}
	if lgc.GenName == "" {
		lgc.GenName = DefaultGenName
	}
	if lgc.Gun == nil && lgc.VU == nil {
		return ErrNoImpl
	}
	if lgc.Schedule == nil {
		return ErrNoSchedule
	}
	if lgc.LoadType != RPS && lgc.LoadType != VU {
		return ErrInvalidScheduleType
	}
	if lgc.LoadType == RPS && lgc.Gun == nil {
		return ErrNoGun
	}
	if lgc.LoadType == VU && lgc.VU == nil {
		return ErrNoVU
	}
	if lgc.RateLimitUnitDuration == 0 {
		lgc.RateLimitUnitDuration = DefaultRateLimitUnitDuration
	}
	return nil
}

// Stats basic generator load stats
type Stats struct {
	// TODO: update json labels with dashboards on major release
	CurrentRPS      atomic.Int64 `json:"currentRPS"`
	CurrentTimeUnit int64        `json:"current_time_unit"`
	CurrentVUs      atomic.Int64 `json:"currentVUs"`
	LastSegment     atomic.Int64 `json:"last_segment"`
	CurrentSegment  atomic.Int64 `json:"current_schedule_segment"`
	SamplesRecorded atomic.Int64 `json:"samples_recorded"`
	SamplesSkipped  atomic.Int64 `json:"samples_skipped"`
	RunStarted      atomic.Bool  `json:"runStarted"`
	RunPaused       atomic.Bool  `json:"runPaused"`
	RunStopped      atomic.Bool  `json:"runStopped"`
	RunFailed       atomic.Bool  `json:"runFailed"`
	Success         atomic.Int64 `json:"success"`
	Failed          atomic.Int64 `json:"failed"`
	CallTimeout     atomic.Int64 `json:"callTimeout"`
	Duration        int64        `json:"load_duration"`
}

// ResponseData includes any request/response data that a gun might store
// ok* slices usually contains successful responses and their verifications if their done async
// fail* slices contains CallResult with response data and an error
type ResponseData struct {
	okDataMu        *sync.Mutex
	OKData          *SliceBuffer[any]
	okResponsesMu   *sync.Mutex
	OKResponses     *SliceBuffer[*Response]
	failResponsesMu *sync.Mutex
	FailResponses   *SliceBuffer[*Response]
}

// Generator generates load with some RPS
type Generator struct {
	Cfg                *Config
	sampler            *Sampler
	Log                zerolog.Logger
	labels             model.LabelSet
	rl                 atomic.Pointer[ratelimit.Limiter]
	scheduleSegments   []*Segment
	currentSegmentMu   *sync.Mutex
	currentSegment     *Segment
	ResponsesWaitGroup *sync.WaitGroup
	dataWaitGroup      *sync.WaitGroup
	ResponsesCtx       context.Context
	responsesCancel    context.CancelFunc
	dataCtx            context.Context
	dataCancel         context.CancelFunc
	gun                Gun
	vu                 VirtualUser
	vus                []VirtualUser
	ResponsesChan      chan *Response
	Responses          *Responses
	responsesData      *ResponseData
	errsMu             *sync.Mutex
	errs               *SliceBuffer[string]
	stats              *Stats
	loki               *LokiClient
	lokiResponsesChan  chan *Response
}

// NewGenerator creates a new generator,
// shoots for scheduled RPS until timeout, test logic is defined through Gun or VirtualUser
func NewGenerator(cfg *Config) (*Generator, error) {
	if cfg == nil {
		return nil, ErrNoCfg
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	for _, s := range cfg.Schedule {
		if err := s.Validate(); err != nil {
			return nil, err
		}
	}
	for _, s := range cfg.Schedule {
		cfg.duration += s.Duration
	}
	l := GetLogger(cfg.T, cfg.GenName)

	ls := LabelsMapToModel(cfg.Labels)
	if cfg.T != nil {
		ls = ls.Merge(model.LabelSet{
			"go_test_name": model.LabelValue(cfg.T.Name()),
		})
	}
	if cfg.GenName != "" {
		ls = ls.Merge(model.LabelSet{
			"gen_name": model.LabelValue(cfg.GenName),
		})
	}
	if err := ls.Validate(); err != nil {
		return nil, ErrInvalidLabels
	}
	cfg.nodeID = os.Getenv("WASP_NODE_ID")
	// context for all requests/responses and vus
	responsesCtx, responsesCancel := context.WithTimeout(context.Background(), cfg.duration)
	// context for all the collected data
	dataCtx, dataCancel := context.WithCancel(context.Background())
	rch := make(chan *Response)
	g := &Generator{
		Cfg:                cfg,
		sampler:            NewSampler(cfg.SamplerConfig),
		scheduleSegments:   cfg.Schedule,
		ResponsesWaitGroup: &sync.WaitGroup{},
		dataWaitGroup:      &sync.WaitGroup{},
		ResponsesCtx:       responsesCtx,
		responsesCancel:    responsesCancel,
		dataCtx:            dataCtx,
		dataCancel:         dataCancel,
		gun:                cfg.Gun,
		vu:                 cfg.VU,
		Responses:          NewResponses(rch),
		ResponsesChan:      rch,
		labels:             ls,
		currentSegmentMu:   &sync.Mutex{},
		responsesData: &ResponseData{
			okDataMu:        &sync.Mutex{},
			OKData:          NewSliceBuffer[any](cfg.CallResultBufLen),
			okResponsesMu:   &sync.Mutex{},
			OKResponses:     NewSliceBuffer[*Response](cfg.CallResultBufLen),
			failResponsesMu: &sync.Mutex{},
			FailResponses:   NewSliceBuffer[*Response](cfg.CallResultBufLen),
		},
		errsMu:            &sync.Mutex{},
		errs:              NewSliceBuffer[string](cfg.CallResultBufLen),
		stats:             &Stats{},
		Log:               l,
		lokiResponsesChan: make(chan *Response, 50000),
	}
	var err error
	if cfg.LokiConfig != nil {
		g.loki, err = NewLokiClient(cfg.LokiConfig)
		if err != nil {
			return nil, err
		}
	}
	CPUCheckLoop()
	return g, nil
}

// runExecuteLoop set up initial data for both RPS and VirtualUser load types
func (g *Generator) runExecuteLoop() {
	g.currentSegment = g.scheduleSegments[0]
	g.stats.LastSegment.Store(int64(len(g.scheduleSegments)))
	switch g.Cfg.LoadType {
	case RPS:
		g.ResponsesWaitGroup.Add(1)
		// we run pacedCall controlled by stats.CurrentRPS
		go func() {
			for {
				select {
				case <-g.ResponsesCtx.Done():
					g.ResponsesWaitGroup.Done()
					g.Log.Info().Msg("RPS generator has stopped")
					return
				default:
					g.pacedCall()
				}
			}
		}()
	case VU:
		g.currentSegmentMu.Lock()
		g.stats.CurrentVUs.Store(g.currentSegment.From)
		g.currentSegmentMu.Unlock()
		// we start all vus once
		vus := g.stats.CurrentVUs.Load()
		for i := 0; i < int(vus); i++ {
			inst := g.vu.Clone(g)
			g.runVU(inst)
			g.vus = append(g.vus, inst)
		}
	}
}

// runSetupWithTimeout runs setup with timeout
func (g *Generator) runSetupWithTimeout(vu VirtualUser) bool {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), g.Cfg.SetupTimeout)
	defer cancel()
	setupChan := make(chan bool)
	go func() {
		if err := vu.Setup(g); err != nil {
			g.ResponsesChan <- &Response{StartedAt: &startedAt, Error: errors.Wrap(err, ErrSetup.Error()).Error(), Failed: true}
			setupChan <- false
		} else {
			setupChan <- true
		}
	}()
	select {
	case <-ctx.Done():
		g.ResponsesChan <- &Response{StartedAt: &startedAt, Error: ErrSetupTimeout.Error(), Timeout: true}
		return false
	case success := <-setupChan:
		return success
	}
}

// runTeardownWithTimeout runs teardown with timeout
func (g *Generator) runTeardownWithTimeout(vu VirtualUser) bool {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), g.Cfg.TeardownTimeout)
	defer cancel()
	setupChan := make(chan bool)
	go func() {
		if err := vu.Teardown(g); err != nil {
			g.ResponsesChan <- &Response{StartedAt: &startedAt, Error: errors.Wrap(err, ErrTeardown.Error()).Error(), Failed: true}
			setupChan <- false
		} else {
			setupChan <- true
		}
	}()
	select {
	case <-ctx.Done():
		g.ResponsesChan <- &Response{StartedAt: &startedAt, Error: ErrTeardownTimeout.Error(), Timeout: true}
		return false
	case success := <-setupChan:
		return success
	}
}

// runVU performs virtual user lifecycle
func (g *Generator) runVU(vu VirtualUser) {
	g.ResponsesWaitGroup.Add(1)
	go func() {
		defer g.ResponsesWaitGroup.Done()
		if !g.runSetupWithTimeout(vu) {
			return
		}
		for {
			if g.stats.RunPaused.Load() {
				continue
			}
			startedAt := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), g.Cfg.CallTimeout)
			vuChan := make(chan struct{})
			go func() {
				vu.Call(g)
				select {
				case <-ctx.Done():
					return
				default:
					vuChan <- struct{}{}
				}
			}()
			select {
			case <-g.ResponsesCtx.Done():
				cancel()
				return
			case <-vu.StopChan():
				g.runTeardownWithTimeout(vu)
				cancel()
				return
			case <-ctx.Done():
				g.ResponsesChan <- &Response{StartedAt: &startedAt, Error: ErrCallTimeout.Error(), Timeout: true}
				cancel()
			case <-vuChan:
			}
		}
	}()
}

// processSegment change RPS or VUs accordingly
// changing both internal and Stats values to report
func (g *Generator) processSegment() bool {
	defer func() {
		g.stats.RunStarted.Store(true)
		g.Log.Info().
			Int64("Segment", g.stats.CurrentSegment.Load()).
			Int64("VUs", g.stats.CurrentVUs.Load()).
			Int64("RPS", g.stats.CurrentRPS.Load()).
			Msg("Schedule segment")
	}()
	if g.stats.CurrentSegment.Load() == g.stats.LastSegment.Load() {
		return true
	}
	g.currentSegmentMu.Lock()
	g.currentSegment = g.scheduleSegments[g.stats.CurrentSegment.Load()]
	g.currentSegmentMu.Unlock()
	g.stats.CurrentSegment.Add(1)
	if g.currentSegment.From == 0 {
		g.currentSegment.From = 1
		g.Pause()
	} else {
		g.Resume()
	}
	switch g.Cfg.LoadType {
	case RPS:
		newRateLimit := ratelimit.New(int(g.currentSegment.From), ratelimit.Per(g.Cfg.RateLimitUnitDuration), ratelimit.WithoutSlack)
		g.rl.Store(&newRateLimit)
		g.stats.CurrentRPS.Store(g.currentSegment.From)
	case VU:
		oldVUs := g.stats.CurrentVUs.Load()
		newVUs := g.currentSegment.From
		g.stats.CurrentVUs.Store(newVUs)

		vusToSpawn := int(math.Abs(float64(max(oldVUs, g.currentSegment.From) - min(oldVUs, g.currentSegment.From))))
		log.Debug().Int64("OldVUs", oldVUs).Int64("NewVUs", newVUs).Int("VUsDelta", vusToSpawn).Msg("Changing VUs")
		if oldVUs == newVUs {
			return false
		}
		if oldVUs > g.currentSegment.From {
			for i := 0; i < vusToSpawn; i++ {
				g.vus[i].Stop(g)
			}
			g.vus = g.vus[vusToSpawn:]
		} else {
			for i := 0; i < vusToSpawn; i++ {
				inst := g.vu.Clone(g)
				g.runVU(inst)
				g.vus = append(g.vus, inst)
			}
		}
	}
	return false
}

// runScheduleLoop runs scheduling loop
// processing segments inside the whole schedule
func (g *Generator) runScheduleLoop() {
	go func() {
		for {
			select {
			case <-g.ResponsesCtx.Done():
				g.Log.Info().
					Int64("Segment", g.stats.CurrentSegment.Load()).
					Int64("VUs", g.stats.CurrentVUs.Load()).
					Int64("RPS", g.stats.CurrentRPS.Load()).
					Msg("Finished all schedule segments")
				g.Log.Info().Msg("Scheduler exited")
				return
			default:
				if g.processSegment() {
					return
				}
				time.Sleep(g.currentSegment.Duration)
			}
		}
	}()
}

// storeResponses stores local metrics for responses, pushed them to Loki stream too if Loki is on
func (g *Generator) storeResponses(res *Response) {
	if g.Cfg.CallTimeout > 0 && res.Duration > g.Cfg.CallTimeout && !res.Timeout {
		return
	}
	if !g.sampler.ShouldRecord(res, g.stats) {
		return
	}
	if g.Cfg.LokiConfig != nil {
		g.lokiResponsesChan <- res
	}
	g.responsesData.okDataMu.Lock()
	g.responsesData.failResponsesMu.Lock()
	g.errsMu.Lock()
	if res.Failed {
		g.stats.RunFailed.Store(true)
		g.stats.Failed.Add(1)
		g.errs.Append(res.Error)
		g.responsesData.FailResponses.Append(res)
		g.Log.Error().Str("Err", res.Error).Msg("load generator request failed")
	} else if res.Timeout {
		g.stats.RunFailed.Store(true)
		g.stats.CallTimeout.Add(1)
		g.stats.Failed.Add(1)
		g.errs.Append(res.Error)
		g.responsesData.FailResponses.Append(res)
		g.Log.Error().Str("Err", res.Error).Msg("load generator request timed out")
	} else {
		g.stats.Success.Add(1)
		g.responsesData.OKData.Append(res.Data)
		g.responsesData.OKResponses.Append(res)
	}
	g.responsesData.okDataMu.Unlock()
	g.responsesData.failResponsesMu.Unlock()
	g.errsMu.Unlock()
	if (g.stats.Failed.Load() > 0 || g.stats.CallTimeout.Load() > 0) && g.Cfg.FailOnErr {
		g.Log.Warn().Msg("Generator has stopped on first error")
		g.responsesCancel()
	}
}

// collectVUResults collects CallResult from all the VUs
func (g *Generator) collectVUResults() {
	if g.Cfg.LoadType == RPS {
		return
	}
	g.dataWaitGroup.Add(1)
	go func() {
		defer g.dataWaitGroup.Done()
		for {
			select {
			case <-g.dataCtx.Done():
				g.Log.Info().Msg("Collect data exited")
				return
			case res := <-g.ResponsesChan:
				if res.StartedAt != nil {
					res.Duration = time.Since(*res.StartedAt)
				}
				tn := time.Now()
				res.FinishedAt = &tn
				g.storeResponses(res)
			}
		}
	}()
}

// pacedCall calls a gun according to a scheduleSegments or plain RPS
func (g *Generator) pacedCall() {
	if !g.Stats().RunStarted.Load() {
		return
	}
	l := *g.rl.Load()
	l.Take()
	if g.stats.RunPaused.Load() {
		return
	}
	if g.stats.RunStopped.Load() {
		return
	}
	result := make(chan *Response)
	requestCtx, cancel := context.WithTimeout(context.Background(), g.Cfg.CallTimeout)
	callStartTS := time.Now()
	go func() {
		result <- g.gun.Call(g)
	}()
	g.ResponsesWaitGroup.Add(1)
	go func() {
		defer g.ResponsesWaitGroup.Done()
		select {
		case <-requestCtx.Done():
			ts := time.Now()
			cr := &Response{Duration: time.Since(callStartTS), FinishedAt: &ts, Timeout: true, Error: ErrCallTimeout.Error()}
			g.storeResponses(cr)
		case res := <-result:
			defer close(result)
			res.Duration = time.Since(callStartTS)
			ts := time.Now()
			res.FinishedAt = &ts
			g.storeResponses(res)
		}
		cancel()
	}()
}

// Run runs load loop until timeout or stop
func (g *Generator) Run(wait bool) (interface{}, bool) {
	g.Log.Info().Msg("Load generator started")
	g.printStatsLoop()
	if g.Cfg.LokiConfig != nil {
		g.sendResponsesToLoki()
		g.sendStatsToLoki()
	}
	g.runScheduleLoop()
	g.runExecuteLoop()
	g.collectVUResults()
	if wait {
		return g.Wait()
	}
	return nil, false
}

// Pause pauses execution of a generator
func (g *Generator) Pause() {
	g.Log.Warn().Msg("Generator was paused")
	g.stats.RunPaused.Store(true)
}

// Resume resumes execution of a generator
func (g *Generator) Resume() {
	g.Log.Warn().Msg("Generator was resumed")
	g.stats.RunPaused.Store(false)
}

// Stop stops load generator, waiting for all calls for either finish or timeout
// this method is external so Gun/VU implementations can stop the generator
func (g *Generator) Stop() (interface{}, bool) {
	if g.stats.RunStopped.Load() {
		return nil, true
	}
	g.stats.RunStarted.Store(false)
	g.stats.RunStopped.Store(true)
	g.stats.RunFailed.Store(true)
	g.Log.Warn().Msg("Graceful stop")
	g.responsesCancel()
	return g.Wait()
}

// Wait waits until test ends
func (g *Generator) Wait() (interface{}, bool) {
	g.Log.Info().Msg("Waiting for all responses to finish")
	g.ResponsesWaitGroup.Wait()
	g.stats.Duration = g.Cfg.duration.Nanoseconds()
	g.stats.CurrentTimeUnit = g.Cfg.RateLimitUnitDuration.Nanoseconds()
	if g.Cfg.LokiConfig != nil {
		g.dataCancel()
		g.dataWaitGroup.Wait()
		g.stopLokiStream()
	}
	return g.GetData(), g.stats.RunFailed.Load()
}

// InputSharedData returns the SharedData passed in Generator config
func (g *Generator) InputSharedData() interface{} {
	return g.Cfg.SharedData
}

// Errors get all calls errors
func (g *Generator) Errors() []string {
	return g.errs.Data
}

// GetData get all calls data
func (g *Generator) GetData() *ResponseData {
	return g.responsesData
}

// Stats get all load stats
func (g *Generator) Stats() *Stats {
	return g.stats
}

/* Loki's methods to handle CallResult/Stats and stream it to Loki */

// stopLokiStream stops the Loki stream client
func (g *Generator) stopLokiStream() {
	if g.Cfg.LokiConfig != nil && g.Cfg.LokiConfig.URL != "" {
		g.Log.Info().Msg("Stopping Loki")
		g.loki.StopNow()
		g.Log.Info().Msg("Loki exited")
	}
}

// handleLokiResponsePayload handles CallResult payload with adding default labels
// adding custom CallResult labels if present
func (g *Generator) handleLokiResponsePayload(r *Response) {
	labels := g.labels.Merge(model.LabelSet{
		"test_data_type": "responses",
		CallGroupLabel:   model.LabelValue(r.Group),
	})
	// we are removing time.Time{} because when it marshalled to string it creates N responses for some Loki queries
	// and to minimize the payload, duration is already calculated at that point
	ts := r.FinishedAt
	r.StartedAt = nil
	r.FinishedAt = nil
	err := g.loki.HandleStruct(labels, *ts, r)
	if err != nil {
		g.Log.Err(err).Send()
		g.Stop()
	}
}

// handleLokiStatsPayload handles StatsJSON payload with adding default labels
// this stream serves as a debug data and shouldn't be customized with additional labels
func (g *Generator) handleLokiStatsPayload() {
	ls := g.labels.Merge(model.LabelSet{
		"test_data_type": "stats",
	})
	err := g.loki.HandleStruct(ls, time.Now(), g.StatsJSON())
	if err != nil {
		g.Log.Err(err).Send()
		g.Stop()
	}
}

// sendResponsesToLoki pushes responses to Loki
func (g *Generator) sendResponsesToLoki() {
	g.Log.Info().
		Str("URL", g.Cfg.LokiConfig.URL).
		Interface("DefaultLabels", g.Cfg.Labels).
		Msg("Streaming data to Loki")
	g.dataWaitGroup.Add(1)
	go func() {
		defer g.dataWaitGroup.Done()
		for {
			select {
			case <-g.dataCtx.Done():
				g.Log.Info().Msg("Loki responses exited")
				return
			case r := <-g.lokiResponsesChan:
				g.handleLokiResponsePayload(r)
			}
		}
	}()
}

// sendStatsToLoki pushes stats to Loki
func (g *Generator) sendStatsToLoki() {
	g.dataWaitGroup.Add(1)
	go func() {
		defer g.dataWaitGroup.Done()
		for {
			select {
			case <-g.dataCtx.Done():
				g.Log.Info().Msg("Loki stats exited")
				return
			default:
				time.Sleep(g.Cfg.StatsPollInterval)
				g.handleLokiStatsPayload()
			}
		}
	}()
}

/* Local logging methods */

// StatsJSON get all load stats for export
func (g *Generator) StatsJSON() map[string]interface{} {
	return map[string]interface{}{
		"node_id":           g.Cfg.nodeID,
		"current_rps":       g.stats.CurrentRPS.Load(),
		"current_instances": g.stats.CurrentVUs.Load(),
		"samples_recorded":  g.stats.SamplesRecorded.Load(),
		"samples_skipped":   g.stats.SamplesSkipped.Load(),
		"run_stopped":       g.stats.RunStopped.Load(),
		"run_failed":        g.stats.RunFailed.Load(),
		"failed":            g.stats.Failed.Load(),
		"success":           g.stats.Success.Load(),
		"callTimeout":       g.stats.CallTimeout.Load(),
		"load_duration":     g.stats.Duration,
		"current_time_unit": g.stats.CurrentTimeUnit,
	}
}

// printStatsLoop prints stats periodically, with Config.StatsPollInterval
func (g *Generator) printStatsLoop() {
	g.ResponsesWaitGroup.Add(1)
	go func() {
		defer g.ResponsesWaitGroup.Done()
		for {
			select {
			case <-g.ResponsesCtx.Done():
				g.Log.Info().Msg("Stats loop exited")
				return
			default:
				time.Sleep(g.Cfg.StatsPollInterval)
				g.Log.Info().
					Int64("Success", g.stats.Success.Load()).
					Int64("Failed", g.stats.Failed.Load()).
					Int64("CallTimeout", g.stats.CallTimeout.Load()).
					Msg("Load stats")
			}
		}
	}()
}

// LabelsMapToModel create model.LabelSet from map of labels
func LabelsMapToModel(m map[string]string) model.LabelSet {
	ls := model.LabelSet{}
	for k, v := range m {
		ls[model.LabelName(k)] = model.LabelValue(v)
	}
	return ls
}
