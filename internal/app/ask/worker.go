package ask

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

type RunJob struct {
	FamilyID     string
	SessionID    string
	RunID        string
	RowVersion   int
	AttemptCount int
}

type RunProcessor interface {
	ProcessRunVersion(context.Context, string, string, string, int) (ExecutionResult, error)
}

type RunEnqueuer interface {
	Enqueue(context.Context, RunJob) error
}

type RunWorkerConfig struct {
	QueueSize        int
	MaxAttempts      int
	RetryDelay       time.Duration
	PollInterval     time.Duration
	LeaseDuration    time.Duration
	ExecutionTimeout time.Duration
	BatchSize        int
	WorkerID         string
	Now              func() time.Time
	OnError          func(RunJob, error)
}

type RunWorker struct {
	processor RunProcessor
	leases    RunLeaseRepository
	jobs      chan RunJob
	stop      chan struct{}
	done      chan struct{}
	config    RunWorkerConfig
	startOnce sync.Once
	closeOnce sync.Once
	enqueueWG sync.WaitGroup
	mu        sync.Mutex
	started   bool
	closed    bool
	pending   map[string]struct{}
}

func NewRunWorker(processor RunProcessor, leases RunLeaseRepository, config RunWorkerConfig) (*RunWorker, error) {
	if processor == nil {
		return nil, errors.New("ask run worker processor is required")
	}
	if leases == nil {
		return nil, errors.New("ask run worker lease repository is required")
	}
	if config.QueueSize < 1 {
		return nil, errors.New("ask run worker queue size must be positive")
	}
	if config.MaxAttempts < 1 {
		return nil, errors.New("ask run worker max attempts must be positive")
	}
	if config.RetryDelay < 0 {
		return nil, errors.New("ask run worker retry delay cannot be negative")
	}
	if config.PollInterval <= 0 || config.LeaseDuration <= 0 || config.ExecutionTimeout <= 0 || config.BatchSize < 1 {
		return nil, errors.New("ask run worker recovery configuration is invalid")
	}
	if config.WorkerID == "" {
		workerID, err := newID()
		if err != nil {
			return nil, err
		}
		config.WorkerID = workerID
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &RunWorker{processor: processor, leases: leases, jobs: make(chan RunJob, config.QueueSize), stop: make(chan struct{}), done: make(chan struct{}), config: config, pending: make(map[string]struct{})}, nil
}

func (w *RunWorker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		w.mu.Lock()
		w.started = true
		w.mu.Unlock()
		go w.run(ctx)
	})
}

func (w *RunWorker) run(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()
	w.recover(ctx)
	for {
		select {
		case <-ctx.Done():
			w.stopQueue()
			return
		case <-w.stop:
			for job := range w.jobs {
				w.process(ctx, job)
			}
			return
		case job, ok := <-w.jobs:
			if !ok {
				return
			}
			w.process(ctx, job)
		case <-ticker.C:
			w.recover(ctx)
		}
	}
}

func (w *RunWorker) process(ctx context.Context, job RunJob) {
	defer w.complete(job)
	now := w.config.Now().UTC()
	claimed, ok, err := w.leases.ClaimRunJob(ctx, job, w.config.WorkerID, now, w.config.LeaseDuration, w.config.MaxAttempts)
	if err != nil {
		w.report(job, err)
		return
	}
	if !ok {
		return
	}
	executionCtx, cancelExecution := context.WithTimeout(ctx, w.config.ExecutionTimeout)
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan error, 1)
	go w.heartbeat(executionCtx, cancelExecution, claimed, stopHeartbeat, heartbeatDone)
	_, err = w.processor.ProcessRunVersion(executionCtx, claimed.FamilyID, claimed.SessionID, claimed.RunID, claimed.RowVersion)
	close(stopHeartbeat)
	<-heartbeatDone
	cancelExecution()
	if err == nil {
		return
	}
	w.report(claimed, err)
	retryNow := w.config.Now().UTC()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cleanupCancel()
	if _, retryErr := w.leases.RetryRunJob(cleanupCtx, claimed, w.config.WorkerID, retryNow, retryNow.Add(w.config.RetryDelay), w.config.MaxAttempts); retryErr != nil {
		w.report(claimed, retryErr)
	}
}

func (w *RunWorker) recover(ctx context.Context) {
	jobs, err := w.leases.ListRunnableRunJobs(ctx, w.config.Now().UTC(), w.config.BatchSize)
	if err != nil {
		w.report(RunJob{}, err)
		return
	}
	for _, job := range jobs {
		w.process(ctx, job)
	}
}

func (w *RunWorker) heartbeat(ctx context.Context, cancel context.CancelFunc, job RunJob, stop <-chan struct{}, done chan<- error) {
	interval := w.config.LeaseDuration / 3
	if interval <= 0 {
		interval = w.config.LeaseDuration
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			done <- ctx.Err()
			return
		case <-stop:
			done <- nil
			return
		case <-ticker.C:
			if err := w.leases.RenewRunLease(ctx, job.RunID, w.config.WorkerID, w.config.Now().UTC().Add(w.config.LeaseDuration)); err != nil {
				cancel()
				done <- err
				return
			}
		}
	}
}

func (w *RunWorker) report(job RunJob, err error) {
	if err != nil && w.config.OnError != nil {
		w.config.OnError(job, err)
	}
}

func (w *RunWorker) Enqueue(ctx context.Context, job RunJob) error {
	if job.FamilyID == "" || job.SessionID == "" || job.RunID == "" || job.RowVersion < 1 {
		return errors.New("ask run worker job is invalid")
	}
	key := runJobKey(job)
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return errors.New("ask run worker is stopped")
	}
	if _, exists := w.pending[key]; exists {
		w.mu.Unlock()
		return nil
	}
	w.pending[key] = struct{}{}
	w.enqueueWG.Add(1)
	w.mu.Unlock()
	defer w.enqueueWG.Done()
	select {
	case <-ctx.Done():
		w.complete(job)
		return ctx.Err()
	case <-w.stop:
		w.complete(job)
		return errors.New("ask run worker is stopped")
	case w.jobs <- job:
		return nil
	}
}

func (w *RunWorker) Close() {
	w.stopQueue()
	w.mu.Lock()
	started := w.started
	w.mu.Unlock()
	if started {
		<-w.done
	}
}

func (w *RunWorker) stopQueue() {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.stop)
		w.mu.Unlock()
		w.enqueueWG.Wait()
		close(w.jobs)
	})
}

func (w *RunWorker) complete(job RunJob) {
	w.mu.Lock()
	delete(w.pending, runJobKey(job))
	w.mu.Unlock()
}

func runJobKey(job RunJob) string {
	return job.FamilyID + ":" + job.SessionID + ":" + job.RunID + ":" + strconv.Itoa(job.RowVersion)
}
