// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package queue

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/log"
)

var manager *Manager

// Manager is a queue manager
type Manager struct {
	mutex sync.Mutex

	counter int64
	Queues  map[int64]*ManagedQueue
}

// ManagedQueue represents a working queue with a Pool of workers.
//
// Although a ManagedQueue should really represent a Queue this does not
// necessarily have to be the case. This could be used to describe any queue.WorkerPool.
type ManagedQueue struct {
	mutex         sync.Mutex
	QID           int64
	Type          Type
	Name          string
	Configuration interface{}
	ExemplarType  string
	Managed       interface{}
	counter       int64
	PoolWorkers   map[int64]*PoolWorkers
}

// Flushable represents a pool or queue that is flushable
type Flushable interface {
	// Flush will add a flush worker to the pool - the worker should be autoregistered with the manager
	Flush(time.Duration) error
	// FlushWithContext is very similar to Flush
	// NB: The worker will not be registered with the manager.
	FlushWithContext(ctx context.Context) error
	// IsEmpty will return if the managed pool is empty and has no work
	IsEmpty() bool
}

// Pausable represents a pool or queue that is Pausable
type Pausable interface {
	// IsPaused will return if the pool or queue is paused
	IsPaused() bool
	// Pause will pause the pool or queue
	Pause()
	// Resume will resume the pool or queue
	Resume()
	// IsPausedIsResumed will return a bool indicating if the pool or queue is paused and a channel that will be closed when it is resumed
	IsPausedIsResumed() (paused, resumed <-chan struct{})
}

// ManagedPool is a simple interface to get certain details from a worker pool
type ManagedPool interface {
	// AddWorkers adds a number of worker as group to the pool with the provided timeout. A CancelFunc is provided to cancel the group
	AddWorkers(number int, timeout time.Duration) context.CancelFunc
	// NumberOfWorkers returns the total number of workers in the pool
	NumberOfWorkers() int
	// MaxNumberOfWorkers returns the maximum number of workers the pool can dynamically grow to
	MaxNumberOfWorkers() int
	// SetMaxNumberOfWorkers sets the maximum number of workers the pool can dynamically grow to
	SetMaxNumberOfWorkers(int)
	// BoostTimeout returns the current timeout for worker groups created during a boost
	BoostTimeout() time.Duration
	// BlockTimeout returns the timeout the internal channel can block for before a boost would occur
	BlockTimeout() time.Duration
	// BoostWorkers sets the number of workers to be created during a boost
	BoostWorkers() int
	// SetPoolSettings sets the user updatable settings for the pool
	SetPoolSettings(maxNumberOfWorkers, boostWorkers int, timeout time.Duration)
	// NumberInQueue returns the total number of items in the pool
	NumberInQueue() int64
	// Done returns a channel that will be closed when the Pool's baseCtx is closed
	Done() <-chan struct{}
}

// ManagedQueueList implements the sort.Interface
type ManagedQueueList []*ManagedQueue

// PoolWorkers represents a group of workers working on a queue
type PoolWorkers struct {
	PID        int64
	Workers    int
	Start      time.Time
	Timeout    time.Time
	HasTimeout bool
	Cancel     context.CancelFunc
	IsFlusher  bool
}

// PoolWorkersList implements the sort.Interface for PoolWorkers
type PoolWorkersList []*PoolWorkers

func init() {
	_ = GetManager()
}

// GetManager returns a Manager and initializes one as singleton if there's none yet
func GetManager() *Manager {
	if manager == nil {
		manager = &Manager{
			Queues: make(map[int64]*ManagedQueue),
		}
	}
	return manager
}

// Add adds a queue to this manager
func (m *Manager) Add(managed interface{},
	t Type,
	configuration,
	exemplar interface{},
) int64 {
	cfg, _ := json.Marshal(configuration)
	mq := &ManagedQueue{
		Type:          t,
		Configuration: string(cfg),
		ExemplarType:  reflect.TypeOf(exemplar).String(),
		PoolWorkers:   make(map[int64]*PoolWorkers),
		Managed:       managed,
	}
	m.mutex.Lock()
	m.counter++
	mq.QID = m.counter
	mq.Name = fmt.Sprintf("queue-%d", mq.QID)
	if named, ok := managed.(Named); ok {
		name := named.Name()
		if len(name) > 0 {
			mq.Name = name
		}
	}
	m.Queues[mq.QID] = mq
	m.mutex.Unlock()
	log.Trace("Queue Manager registered: %s (QID: %d)", mq.Name, mq.QID)
	return mq.QID
}

// Remove a queue from the Manager
func (m *Manager) Remove(qid int64) {
	m.mutex.Lock()
	delete(m.Queues, qid)
	m.mutex.Unlock()
	log.Trace("Queue Manager removed: QID: %d", qid)
}

// GetManagedQueue by qid
func (m *Manager) GetManagedQueue(qid int64) *ManagedQueue {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.Queues[qid]
}

// FlushAll flushes all the flushable queues attached to this manager
func (m *Manager) FlushAll(baseCtx context.Context, timeout time.Duration) error {
	var ctx context.Context
	var cancel context.CancelFunc
	start := time.Now()
	end := start
	hasTimeout := false
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(baseCtx, timeout)
		end = start.Add(timeout)
		hasTimeout = true
	} else {
		ctx, cancel = context.WithCancel(baseCtx)
	}
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			mqs := m.ManagedQueues()
			nonEmptyQueues := []string{}
			for _, mq := range mqs {
				if !mq.IsEmpty() {
					nonEmptyQueues = append(nonEmptyQueues, mq.Name)
				}
			}
			if len(nonEmptyQueues) > 0 {
				return fmt.Errorf("flush timeout with non-empty queues: %s", strings.Join(nonEmptyQueues, ", "))
			}
			return nil
		default:
		}
		mqs := m.ManagedQueues()
		log.Debug("Found %d Managed Queues", len(mqs))
		wg := sync.WaitGroup{}
		wg.Add(len(mqs))
		allEmpty := true
		for _, mq := range mqs {
			if mq.IsEmpty() {
				wg.Done()
				continue
			}
			if pausable, ok := mq.Managed.(Pausable); ok {
				// no point flushing paused queues
				if pausable.IsPaused() {
					wg.Done()
					continue
				}
			}
			if pool, ok := mq.Managed.(ManagedPool); ok {
				// No point into flushing pools when their base's ctx is already done.
				select {
				case <-pool.Done():
					wg.Done()
					continue
				default:
				}
			}

			allEmpty = false
			if flushable, ok := mq.Managed.(Flushable); ok {
				log.Debug("Flushing (flushable) queue: %s", mq.Name)
				go func(q *ManagedQueue) {
					localCtx, localCtxCancel := context.WithCancel(ctx)
					pid := q.RegisterWorkers(1, start, hasTimeout, end, localCtxCancel, true)
					err := flushable.FlushWithContext(localCtx)
					if err != nil && err != ctx.Err() {
						cancel()
					}
					q.CancelWorkers(pid)
					localCtxCancel()
					wg.Done()
				}(mq)
			} else {
				log.Debug("Queue: %s is non-empty but is not flushable", mq.Name)
				wg.Done()
			}
		}
		if allEmpty {
			log.Debug("All queues are empty")
			break
		}
		// Ensure there are always at least 100ms between loops but not more if we've actually been doing some flushing
		// but don't delay cancellation here.
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
		wg.Wait()
	}
	return nil
}

// ManagedQueues returns the managed queues
func (m *Manager) ManagedQueues() []*ManagedQueue {
	m.mutex.Lock()
	mqs := make([]*ManagedQueue, 0, len(m.Queues))
	for _, mq := range m.Queues {
		mqs = append(mqs, mq)
	}
	m.mutex.Unlock()
	sort.Sort(ManagedQueueList(mqs))
	return mqs
}

// Workers returns the poolworkers
func (q *ManagedQueue) Workers() []*PoolWorkers {
	q.mutex.Lock()
	workers := make([]*PoolWorkers, 0, len(q.PoolWorkers))
	for _, worker := range q.PoolWorkers {
		workers = append(workers, worker)
	}
	q.mutex.Unlock()
	sort.Sort(PoolWorkersList(workers))
	return workers
}

// RegisterWorkers registers workers to this queue
func (q *ManagedQueue) RegisterWorkers(number int, start time.Time, hasTimeout bool, timeout time.Time, cancel context.CancelFunc, isFlusher bool) int64 {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.counter++
	q.PoolWorkers[q.counter] = &PoolWorkers{
		PID:        q.counter,
		Workers:    number,
		Start:      start,
		Timeout:    timeout,
		HasTimeout: hasTimeout,
		Cancel:     cancel,
		IsFlusher:  isFlusher,
	}
	return q.counter
}

// CancelWorkers cancels pooled workers with pid
func (q *ManagedQueue) CancelWorkers(pid int64) {
	q.mutex.Lock()
	pw, ok := q.PoolWorkers[pid]
	q.mutex.Unlock()
	if !ok {
		return
	}
	pw.Cancel()
}

// RemoveWorkers deletes pooled workers with pid
func (q *ManagedQueue) RemoveWorkers(pid int64) {
	q.mutex.Lock()
	pw, ok := q.PoolWorkers[pid]
	delete(q.PoolWorkers, pid)
	q.mutex.Unlock()
	if ok && pw.Cancel != nil {
		pw.Cancel()
	}
}

// AddWorkers adds workers to the queue if it has registered an add worker function
func (q *ManagedQueue) AddWorkers(number int, timeout time.Duration) context.CancelFunc {
	if pool, ok := q.Managed.(ManagedPool); ok {
		// the cancel will be added to the pool workers description above
		return pool.AddWorkers(number, timeout)
	}
	return nil
}

// Flushable returns true if the queue is flushable
func (q *ManagedQueue) Flushable() bool {
	_, ok := q.Managed.(Flushable)
	return ok
}

// Flush flushes the queue with a timeout
func (q *ManagedQueue) Flush(timeout time.Duration) error {
	if flushable, ok := q.Managed.(Flushable); ok {
		// the cancel will be added to the pool workers description above
		return flushable.Flush(timeout)
	}
	return nil
}

// IsEmpty returns if the queue is empty
func (q *ManagedQueue) IsEmpty() bool {
	if flushable, ok := q.Managed.(Flushable); ok {
		return flushable.IsEmpty()
	}
	return true
}

// Pausable returns whether the queue is Pausable
func (q *ManagedQueue) Pausable() bool {
	_, ok := q.Managed.(Pausable)
	return ok
}

// Pause pauses the queue
func (q *ManagedQueue) Pause() {
	if pausable, ok := q.Managed.(Pausable); ok {
		pausable.Pause()
	}
}

// IsPaused reveals if the queue is paused
func (q *ManagedQueue) IsPaused() bool {
	if pausable, ok := q.Managed.(Pausable); ok {
		return pausable.IsPaused()
	}
	return false
}

// Resume resumes the queue
func (q *ManagedQueue) Resume() {
	if pausable, ok := q.Managed.(Pausable); ok {
		pausable.Resume()
	}
}

// NumberOfWorkers returns the number of workers in the queue
func (q *ManagedQueue) NumberOfWorkers() int {
	if pool, ok := q.Managed.(ManagedPool); ok {
		return pool.NumberOfWorkers()
	}
	return -1
}

// MaxNumberOfWorkers returns the maximum number of workers for the pool
func (q *ManagedQueue) MaxNumberOfWorkers() int {
	if pool, ok := q.Managed.(ManagedPool); ok {
		return pool.MaxNumberOfWorkers()
	}
	return 0
}

// BoostWorkers returns the number of workers for a boost
func (q *ManagedQueue) BoostWorkers() int {
	if pool, ok := q.Managed.(ManagedPool); ok {
		return pool.BoostWorkers()
	}
	return -1
}

// BoostTimeout returns the timeout of the next boost
func (q *ManagedQueue) BoostTimeout() time.Duration {
	if pool, ok := q.Managed.(ManagedPool); ok {
		return pool.BoostTimeout()
	}
	return 0
}

// BlockTimeout returns the timeout til the next boost
func (q *ManagedQueue) BlockTimeout() time.Duration {
	if pool, ok := q.Managed.(ManagedPool); ok {
		return pool.BlockTimeout()
	}
	return 0
}

// SetPoolSettings sets the setable boost values
func (q *ManagedQueue) SetPoolSettings(maxNumberOfWorkers, boostWorkers int, timeout time.Duration) {
	if pool, ok := q.Managed.(ManagedPool); ok {
		pool.SetPoolSettings(maxNumberOfWorkers, boostWorkers, timeout)
	}
}

// NumberInQueue returns the number of items in the queue
func (q *ManagedQueue) NumberInQueue() int64 {
	if pool, ok := q.Managed.(ManagedPool); ok {
		return pool.NumberInQueue()
	}
	return -1
}

func (l ManagedQueueList) Len() int {
	return len(l)
}

func (l ManagedQueueList) Less(i, j int) bool {
	return l[i].Name < l[j].Name
}

func (l ManagedQueueList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l PoolWorkersList) Len() int {
	return len(l)
}

func (l PoolWorkersList) Less(i, j int) bool {
	return l[i].Start.Before(l[j].Start)
}

func (l PoolWorkersList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
