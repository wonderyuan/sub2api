package service

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	userConcurrencyTrendFlushInterval = time.Second
	userConcurrencyTrendSampleTimeout = 3 * time.Second
	userConcurrencyTrendEventBuffer   = 4096
)

// ConcurrencyPeak is the peak concurrency observed within one minute.
type ConcurrencyPeak struct {
	PeakInUse   int `json:"peak_in_use"`
	PeakWaiting int `json:"peak_waiting"`
	PeakDemand  int `json:"peak_demand"`
}

type ConcurrencySnapshot struct {
	InUse   int `json:"in_use"`
	Waiting int `json:"waiting"`
	Demand  int `json:"demand"`
}

type UserConcurrencyTrendPoint struct {
	BucketStart time.Time                 `json:"bucket_start"`
	System      ConcurrencyPeak           `json:"system"`
	Users       map[int64]ConcurrencyPeak `json:"users"`
}

type UserConcurrencyTrend struct {
	StartTime time.Time                   `json:"start_time"`
	EndTime   time.Time                   `json:"end_time"`
	Bucket    string                      `json:"bucket"`
	Points    []UserConcurrencyTrendPoint `json:"points"`
}

// UserConcurrencyTrendCache is implemented by Redis-backed concurrency caches.
// It is optional so lightweight test stubs and alternative caches remain compatible.
type UserConcurrencyTrendCache interface {
	GetActiveUserLoads(ctx context.Context) (map[int64]*UserLoadInfo, error)
	MergeUserConcurrencyTrend(ctx context.Context, bucketStart time.Time, users map[int64]ConcurrencyPeak, system ConcurrencyPeak) error
	GetUserConcurrencyTrend(ctx context.Context, start, end time.Time) (*UserConcurrencyTrend, error)
}

type userConcurrencyStateCache interface {
	TrackUserSlotWithState(ctx context.Context, userID int64, requestID string) (int, time.Time, error)
	AcquireUserSlotWithState(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, int, time.Time, error)
	ReleaseUserSlotWithState(ctx context.Context, userID int64, requestID string) (int, time.Time, error)
	IncrementWaitCountWithState(ctx context.Context, userID int64, maxWait int) (bool, int, time.Time, error)
	DecrementWaitCountWithState(ctx context.Context, userID int64) (int, time.Time, error)
}

type userConcurrencyStateEvent struct {
	userID  int64
	active  *int
	waiting *int
	at      time.Time
}

type userConcurrencyLiveState struct {
	active  int
	waiting int
}

type userConcurrencyTrendRecorder struct {
	cache UserConcurrencyTrendCache

	events   chan userConcurrencyStateEvent
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
	dropped  atomic.Uint64
}

func newUserConcurrencyTrendRecorder(cache ConcurrencyCache) *userConcurrencyTrendRecorder {
	trendCache, ok := cache.(UserConcurrencyTrendCache)
	if !ok || trendCache == nil {
		return nil
	}
	r := &userConcurrencyTrendRecorder{
		cache:  trendCache,
		events: make(chan userConcurrencyStateEvent, userConcurrencyTrendEventBuffer),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *userConcurrencyTrendRecorder) observe(event userConcurrencyStateEvent) {
	if r == nil || event.userID <= 0 {
		return
	}
	if event.at.IsZero() {
		event.at = time.Now().UTC()
	}
	select {
	case r.events <- event:
	default:
		r.dropped.Add(1)
	}
}

func (r *userConcurrencyTrendRecorder) stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.stopCh) })
	<-r.doneCh
}

func (r *userConcurrencyTrendRecorder) run() {
	defer close(r.doneCh)

	live := make(map[int64]userConcurrencyLiveState)
	pending := make(map[time.Time]map[int64]ConcurrencyPeak)
	systemPending := make(map[time.Time]ConcurrencyPeak)
	totalActive := 0
	totalWaiting := 0

	flushTicker := time.NewTicker(userConcurrencyTrendFlushInterval)
	defer flushTicker.Stop()

	reconcile := func(now time.Time) {
		ctx, cancel := context.WithTimeout(context.Background(), userConcurrencyTrendSampleTimeout)
		loads, err := r.cache.GetActiveUserLoads(ctx)
		cancel()
		if err != nil {
			log.Printf("[ConcurrencyTrend] reconcile active users failed: %v", err)
			return
		}

		live = make(map[int64]userConcurrencyLiveState, len(loads))
		totalActive = 0
		totalWaiting = 0
		for userID, load := range loads {
			if load == nil || (load.CurrentConcurrency <= 0 && load.WaitingCount <= 0) {
				continue
			}
			state := userConcurrencyLiveState{
				active:  max(load.CurrentConcurrency, 0),
				waiting: max(load.WaitingCount, 0),
			}
			live[userID] = state
			totalActive += state.active
			totalWaiting += state.waiting
		}
		recordConcurrencyTrendSample(pending, systemPending, now, live, 0, totalActive, totalWaiting)
	}

	flush := func() {
		for bucket, users := range pending {
			ctx, cancel := context.WithTimeout(context.Background(), userConcurrencyTrendSampleTimeout)
			err := r.cache.MergeUserConcurrencyTrend(ctx, bucket, users, systemPending[bucket])
			cancel()
			if err != nil {
				log.Printf("[ConcurrencyTrend] flush bucket %s failed: %v", bucket.Format(time.RFC3339), err)
				continue
			}
			delete(pending, bucket)
			delete(systemPending, bucket)
		}
	}

	reconcile(time.Now().UTC())
	for {
		select {
		case event := <-r.events:
			state := live[event.userID]
			if event.active != nil {
				next := max(*event.active, 0)
				totalActive += next - state.active
				state.active = next
			}
			if event.waiting != nil {
				next := max(*event.waiting, 0)
				totalWaiting += next - state.waiting
				state.waiting = next
			}
			if state.active == 0 && state.waiting == 0 {
				delete(live, event.userID)
			} else {
				live[event.userID] = state
			}
			recordConcurrencyTrendSample(pending, systemPending, event.at, live, event.userID, totalActive, totalWaiting)
		case now := <-flushTicker.C:
			reconcile(now.UTC())
			flush()
			if dropped := r.dropped.Swap(0); dropped > 0 {
				log.Printf("[ConcurrencyTrend] dropped %d realtime events; one-second reconciliation applied", dropped)
			}
		case <-r.stopCh:
			reconcile(time.Now().UTC())
			flush()
			return
		}
	}
}

func recordConcurrencyTrendSample(
	pending map[time.Time]map[int64]ConcurrencyPeak,
	systemPending map[time.Time]ConcurrencyPeak,
	at time.Time,
	live map[int64]userConcurrencyLiveState,
	changedUserID int64,
	totalActive int,
	totalWaiting int,
) {
	if totalActive <= 0 && totalWaiting <= 0 && len(live) == 0 {
		return
	}
	bucket := at.UTC().Truncate(time.Minute)
	users := pending[bucket]
	if users == nil {
		users = make(map[int64]ConcurrencyPeak)
		pending[bucket] = users
	}
	recordUser := func(userID int64, state userConcurrencyLiveState) {
		if state.active <= 0 && state.waiting <= 0 {
			return
		}
		peak := users[userID]
		peak.PeakInUse = max(peak.PeakInUse, state.active)
		peak.PeakWaiting = max(peak.PeakWaiting, state.waiting)
		peak.PeakDemand = max(peak.PeakDemand, state.active+state.waiting)
		users[userID] = peak
	}
	if changedUserID > 0 {
		if state, ok := live[changedUserID]; ok {
			recordUser(changedUserID, state)
		}
	} else {
		for userID, state := range live {
			recordUser(userID, state)
		}
	}

	system := systemPending[bucket]
	system.PeakInUse = max(system.PeakInUse, totalActive)
	system.PeakWaiting = max(system.PeakWaiting, totalWaiting)
	system.PeakDemand = max(system.PeakDemand, totalActive+totalWaiting)
	systemPending[bucket] = system
}

func (s *ConcurrencyService) observeUserConcurrencyState(userID int64, active, waiting *int, at time.Time) {
	if s == nil || s.trendRecorder == nil {
		return
	}
	s.trendRecorder.observe(userConcurrencyStateEvent{userID: userID, active: active, waiting: waiting, at: at})
}

func (s *ConcurrencyService) GetUserConcurrencyTrend(ctx context.Context, now time.Time) (*UserConcurrencyTrend, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	end := now.UTC().Truncate(time.Minute)
	start := end.Add(-59 * time.Minute)
	if s == nil || s.trendRecorder == nil || s.trendRecorder.cache == nil {
		points := make([]UserConcurrencyTrendPoint, 0, 60)
		for bucket := start; !bucket.After(end); bucket = bucket.Add(time.Minute) {
			points = append(points, UserConcurrencyTrendPoint{BucketStart: bucket, Users: map[int64]ConcurrencyPeak{}})
		}
		return &UserConcurrencyTrend{StartTime: start, EndTime: end, Bucket: "minute", Points: points}, nil
	}
	return s.trendRecorder.cache.GetUserConcurrencyTrend(ctx, start, end)
}

func (s *ConcurrencyService) GetCurrentUserConcurrencyLoads(ctx context.Context) (map[int64]*UserLoadInfo, error) {
	if s == nil || s.trendRecorder == nil || s.trendRecorder.cache == nil {
		return map[int64]*UserLoadInfo{}, nil
	}
	return s.trendRecorder.cache.GetActiveUserLoads(ctx)
}
