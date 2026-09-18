package ask

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRunWorkerValidatesConfigurationAndJobs(t *testing.T) {
	processor := &workerTestProcessor{}
	leases := &workerTestLeaseRepository{}
	config := workerTestConfig()
	if _, err := NewRunWorker(nil, leases, config); err == nil {
		t.Fatal("nil processor error = nil")
	}
	if _, err := NewRunWorker(processor, nil, config); err == nil {
		t.Fatal("nil lease repository error = nil")
	}
	invalid := config
	invalid.QueueSize = 0
	if _, err := NewRunWorker(processor, leases, invalid); err == nil {
		t.Fatal("invalid queue size error = nil")
	}
	invalid = config
	invalid.MaxAttempts = 0
	if _, err := NewRunWorker(processor, leases, invalid); err == nil {
		t.Fatal("invalid max attempts error = nil")
	}
	invalid = config
	invalid.RetryDelay = -time.Second
	if _, err := NewRunWorker(processor, leases, invalid); err == nil {
		t.Fatal("invalid retry delay error = nil")
	}
	invalid = config
	invalid.LeaseDuration = 0
	if _, err := NewRunWorker(processor, leases, invalid); err == nil {
		t.Fatal("invalid recovery configuration error = nil")
	}
	worker, err := NewRunWorker(processor, leases, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Enqueue(context.Background(), RunJob{}); err == nil {
		t.Fatal("invalid job error = nil")
	}
	worker.Close()
}

func TestRunWorkerDeduplicatesExecutionVersionAndDrainsOnClose(t *testing.T) {
	processor := &workerTestProcessor{started: make(chan string, 2), release: make(chan struct{})}
	leases := &workerTestLeaseRepository{}
	worker, err := NewRunWorker(processor, leases, workerTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	worker.Start(context.Background())
	worker.Start(context.Background())
	first := RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-1", RowVersion: 1}
	if err := worker.Enqueue(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if runID := receiveWorkerValue(t, processor.started); runID != first.RunID {
		t.Fatalf("started run = %s", runID)
	}
	if err := worker.Enqueue(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.RowVersion = 4
	if err := worker.Enqueue(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() {
		worker.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("worker closed before current job completed")
	case <-time.After(20 * time.Millisecond):
	}
	processor.release <- struct{}{}
	if runID := receiveWorkerValue(t, processor.started); runID != second.RunID {
		t.Fatalf("started queued run = %s", runID)
	}
	processor.release <- struct{}{}
	receiveWorkerValue(t, closed)
	worker.Close()
	if calls := processor.callCount(); calls != 2 {
		t.Fatalf("process calls = %d, want 2", calls)
	}
}

func TestRunWorkerSchedulesPersistentRetry(t *testing.T) {
	processor := &workerTestProcessor{failures: 1, started: make(chan string, 1)}
	leases := &workerTestLeaseRepository{}
	errorsReported := make(chan error, 1)
	config := workerTestConfig()
	config.OnError = func(_ RunJob, err error) {
		errorsReported <- err
	}
	worker, err := NewRunWorker(processor, leases, config)
	if err != nil {
		t.Fatal(err)
	}
	worker.Start(context.Background())
	job := RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-1", RowVersion: 1}
	if err := worker.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	receiveWorkerValue(t, processor.started)
	if err := receiveWorkerValue(t, errorsReported); err.Error() != "temporary failure" {
		t.Fatalf("worker error = %v", err)
	}
	retry := receiveWorkerValue(t, leases.retriedJobs())
	if retry.RunID != job.RunID || retry.AttemptCount != 1 {
		t.Fatalf("retried job = %+v", retry)
	}
	worker.Close()
}

func TestRunWorkerUsesProviderRetryDelay(t *testing.T) {
	processor := &workerTestProcessor{errorValue: NewExecutorError("provider_rate_limited", true, 7*time.Second, errors.New("rate limited")), started: make(chan string, 1)}
	leases := &workerTestLeaseRepository{}
	config := workerTestConfig()
	config.Now = func() time.Time { return time.Unix(100, 0).UTC() }
	worker, err := NewRunWorker(processor, leases, config)
	if err != nil {
		t.Fatal(err)
	}
	worker.Start(context.Background())
	job := RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-1", RowVersion: 1}
	if err := worker.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	retryAt := receiveWorkerValue(t, leases.retryTimes())
	if !retryAt.Equal(time.Unix(107, 0).UTC()) {
		t.Fatalf("retry at = %s", retryAt)
	}
	worker.Close()
}

func TestRunWorkerRecoversPersistedJobsOnStart(t *testing.T) {
	job := RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-1", RowVersion: 1}
	processor := &workerTestProcessor{started: make(chan string, 1)}
	leases := &workerTestLeaseRepository{runnable: []RunJob{job}}
	worker, err := NewRunWorker(processor, leases, workerTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	worker.Start(context.Background())
	if runID := receiveWorkerValue(t, processor.started); runID != job.RunID {
		t.Fatalf("recovered run = %s", runID)
	}
	worker.Close()
}

func TestRunWorkerRenewsLeaseDuringExecution(t *testing.T) {
	processor := &workerTestProcessor{started: make(chan string, 1), release: make(chan struct{})}
	leases := &workerTestLeaseRepository{renewed: make(chan string, 1)}
	config := workerTestConfig()
	config.LeaseDuration = 30 * time.Millisecond
	worker, err := NewRunWorker(processor, leases, config)
	if err != nil {
		t.Fatal(err)
	}
	worker.Start(context.Background())
	job := RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-1", RowVersion: 1}
	if err := worker.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	receiveWorkerValue(t, processor.started)
	if runID := receiveWorkerValue(t, leases.renewed); runID != job.RunID {
		t.Fatalf("renewed run = %s", runID)
	}
	processor.release <- struct{}{}
	worker.Close()
}

func TestRunWorkerStopsExecutionAtTimeout(t *testing.T) {
	leases := &workerTestLeaseRepository{}
	config := workerTestConfig()
	config.ExecutionTimeout = 10 * time.Millisecond
	worker, err := NewRunWorker(workerContextProcessor{}, leases, config)
	if err != nil {
		t.Fatal(err)
	}
	worker.Start(context.Background())
	job := RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-1", RowVersion: 1}
	if err := worker.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if retry := receiveWorkerValue(t, leases.retriedJobs()); retry.RunID != job.RunID {
		t.Fatalf("timed out run = %+v", retry)
	}
	worker.Close()
}

func TestRunWorkerEnqueueRespectsCancellation(t *testing.T) {
	worker, err := NewRunWorker(&workerTestProcessor{}, &workerTestLeaseRepository{}, workerTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Enqueue(context.Background(), RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-1", RowVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err := worker.Enqueue(context.Background(), RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-2", RowVersion: 1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = worker.Enqueue(ctx, RunJob{FamilyID: "family-1", SessionID: "session-1", RunID: "run-3", RowVersion: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("enqueue error = %v, want context canceled", err)
	}
	worker.Close()
}

func workerTestConfig() RunWorkerConfig {
	return RunWorkerConfig{QueueSize: 2, MaxAttempts: 3, RetryDelay: time.Millisecond, PollInterval: time.Hour, LeaseDuration: time.Hour, ExecutionTimeout: time.Hour, BatchSize: 2, WorkerID: "worker-1"}
}

type workerTestProcessor struct {
	mu         sync.Mutex
	calls      int
	failures   int
	errorValue error
	started    chan string
	release    chan struct{}
}

type workerContextProcessor struct{}

func (workerContextProcessor) ProcessRunVersion(ctx context.Context, _, _, _ string, _, _ int) (ExecutionResult, error) {
	<-ctx.Done()
	return ExecutionResult{}, ctx.Err()
}

func (p *workerTestProcessor) ProcessRunVersion(_ context.Context, _, _, runID string, _, _ int) (ExecutionResult, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if p.started != nil {
		p.started <- runID
	}
	if p.release != nil {
		<-p.release
	}
	if call <= p.failures {
		return ExecutionResult{}, errors.New("temporary failure")
	}
	if p.errorValue != nil {
		return ExecutionResult{}, p.errorValue
	}
	return ExecutionResult{}, nil
}

func (p *workerTestProcessor) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type workerTestLeaseRepository struct {
	mu       sync.Mutex
	runnable []RunJob
	retried  chan RunJob
	renewed  chan string
	retryAt  chan time.Time
}

func (r *workerTestLeaseRepository) ListRunnableRunJobs(context.Context, time.Time, int) ([]RunJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := append([]RunJob(nil), r.runnable...)
	r.runnable = nil
	return result, nil
}

func (r *workerTestLeaseRepository) ClaimRunJob(_ context.Context, job RunJob, _ string, _ time.Time, _ time.Duration, _ int) (RunJob, bool, error) {
	job.AttemptCount++
	job.ExecutionEpoch++
	return job, true, nil
}

func (r *workerTestLeaseRepository) RenewRunLease(_ context.Context, runID, _ string, _ time.Time) error {
	if r.renewed != nil {
		r.renewed <- runID
	}
	return nil
}

func (r *workerTestLeaseRepository) RetryRunJob(_ context.Context, job RunJob, _ string, _, nextAttemptAt time.Time, _ int) (bool, error) {
	r.retriedJobs() <- job
	select {
	case r.retryTimes() <- nextAttemptAt:
	default:
	}
	return false, nil
}

func (r *workerTestLeaseRepository) retryTimes() chan time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.retryAt == nil {
		r.retryAt = make(chan time.Time, 1)
	}
	return r.retryAt
}

func (r *workerTestLeaseRepository) retriedJobs() chan RunJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.retried == nil {
		r.retried = make(chan RunJob, 1)
	}
	return r.retried
}

func receiveWorkerValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker")
		var zero T
		return zero
	}
}
