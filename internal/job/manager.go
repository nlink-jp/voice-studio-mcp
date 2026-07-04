// Package job runs batch synthesis jobs asynchronously and tracks their
// progress in memory.
//
// Jobs are deliberately NOT persisted: after a server restart check_job
// reports job_not_found with recovery guidance, and re-running
// synthesize_script is cheap because the synthesis cache skips every
// unchanged line.
package job

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

// States of a job. Per-line failures do not fail a job; it still ends "done"
// with a failure list. "failed" is reserved for job-level fatalities
// (server shutdown mid-run).
const (
	StateRunning = "running"
	StateDone    = "done"
	StateFailed  = "failed"
)

// maxFinishedJobs bounds the finished-job history kept in memory.
const maxFinishedJobs = 32

// maxReportedFailures bounds the failures attached to one status response.
const maxReportedFailures = 20

// Failure describes one failed line.
type Failure struct {
	LineID  int    `json:"line_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Item is one unit of work: a line id plus the closure that renders it.
type Item struct {
	LineID int
	// Run synthesizes the line and reports whether it was a cache hit.
	Run func(ctx context.Context) (cached bool, err error)
}

// Status is the check_job view of a job.
type Status struct {
	JobID             string    `json:"job_id"`
	State             string    `json:"state"`
	Total             int       `json:"total"`
	Done              int       `json:"done"`
	Cached            int       `json:"cached"`
	Failed            int       `json:"failed"`
	Failures          []Failure `json:"failures,omitempty"`
	FailuresTruncated bool      `json:"failures_truncated,omitempty"`
	FatalError        string    `json:"fatal_error,omitempty"`
	StartedAt         string    `json:"started_at"`
	FinishedAt        string    `json:"finished_at,omitempty"`
	OutputDir         string    `json:"output_dir"`
}

type jobState struct {
	id        string
	outputDir string
	total     int

	mu         sync.Mutex
	done       int
	cached     int
	failures   []Failure
	state      string
	fatal      string
	startedAt  time.Time
	finishedAt time.Time
}

// Manager owns the job table and the synthesis concurrency budget. The
// semaphore is shared across jobs so parallel jobs cannot overload the
// engine.
type Manager struct {
	sem chan struct{}

	mu   sync.Mutex
	jobs map[string]*jobState
}

// NewManager creates a Manager allowing `concurrency` simultaneous line
// syntheses (minimum 1).
func NewManager(concurrency int) *Manager {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Manager{
		sem:  make(chan struct{}, concurrency),
		jobs: make(map[string]*jobState),
	}
}

// Submit starts a job over items and returns its id immediately.
// ctx should be the server's lifetime context: cancellation aborts the
// remaining lines and marks the job failed with recovery guidance.
func (m *Manager) Submit(ctx context.Context, outputDir string, items []Item) string {
	id := "job_" + randomHex(8)
	js := &jobState{
		id:        id,
		outputDir: outputDir,
		total:     len(items),
		state:     StateRunning,
		startedAt: time.Now(),
	}
	m.mu.Lock()
	m.jobs[id] = js
	m.evictLocked()
	m.mu.Unlock()

	go m.run(ctx, js, items)
	return id
}

func (m *Manager) run(ctx context.Context, js *jobState, items []Item) {
	var wg sync.WaitGroup
	for _, it := range items {
		select {
		case <-ctx.Done():
			js.finish(StateFailed, "server shut down while the job was running; re-run synthesize_script — cached lines are skipped automatically")
			return
		case m.sem <- struct{}{}:
		}
		wg.Add(1)
		go func(it Item) {
			defer wg.Done()
			defer func() { <-m.sem }()
			cached, err := it.Run(ctx)
			js.record(it.LineID, cached, err)
		}(it)
	}
	wg.Wait()
	if ctx.Err() != nil {
		js.finish(StateFailed, "server shut down while the job was running; re-run synthesize_script — cached lines are skipped automatically")
		return
	}
	js.finish(StateDone, "")
}

func (js *jobState) record(lineID int, cached bool, err error) {
	js.mu.Lock()
	defer js.mu.Unlock()
	if err != nil {
		f := Failure{LineID: lineID, Code: "error", Message: err.Error()}
		var te *toolerr.Error
		if errors.As(err, &te) {
			f.Code = te.Code
			f.Message = te.Message
		}
		js.failures = append(js.failures, f)
		return
	}
	js.done++
	if cached {
		js.cached++
	}
}

func (js *jobState) finish(state, fatal string) {
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.state != StateRunning {
		return
	}
	js.state = state
	js.fatal = fatal
	js.finishedAt = time.Now()
}

// Get returns the status of a job, or a job_not_found error that tells the
// agent how to recover after a server restart.
func (m *Manager) Get(jobID string) (Status, error) {
	m.mu.Lock()
	js, ok := m.jobs[jobID]
	m.mu.Unlock()
	if !ok {
		return Status{}, toolerr.Newf(toolerr.CodeJobNotFound,
			"job %q not found — jobs do not survive a server restart; re-run synthesize_script (cached lines are skipped, so only missing lines are synthesized)", jobID)
	}

	js.mu.Lock()
	defer js.mu.Unlock()
	st := Status{
		JobID:      js.id,
		State:      js.state,
		Total:      js.total,
		Done:       js.done,
		Cached:     js.cached,
		Failed:     len(js.failures),
		OutputDir:  js.outputDir,
		FatalError: js.fatal,
		StartedAt:  js.startedAt.Format(time.RFC3339),
	}
	if !js.finishedAt.IsZero() {
		st.FinishedAt = js.finishedAt.Format(time.RFC3339)
	}
	failures := js.failures
	if len(failures) > maxReportedFailures {
		failures = failures[:maxReportedFailures]
		st.FailuresTruncated = true
	}
	st.Failures = append([]Failure(nil), failures...)
	sort.Slice(st.Failures, func(i, j int) bool { return st.Failures[i].LineID < st.Failures[j].LineID })
	return st, nil
}

// evictLocked drops the oldest finished jobs beyond maxFinishedJobs.
// Caller holds m.mu.
func (m *Manager) evictLocked() {
	type fin struct {
		id string
		at time.Time
	}
	var finished []fin
	for id, js := range m.jobs {
		js.mu.Lock()
		if js.state != StateRunning {
			finished = append(finished, fin{id, js.finishedAt})
		}
		js.mu.Unlock()
	}
	if len(finished) <= maxFinishedJobs {
		return
	}
	sort.Slice(finished, func(i, j int) bool { return finished[i].at.Before(finished[j].at) })
	for _, f := range finished[:len(finished)-maxFinishedJobs] {
		delete(m.jobs, f.id)
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is unrecoverable in practice; a time-based
		// fallback keeps ids unique enough for an in-memory table.
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000000")))[:2*n]
	}
	return hex.EncodeToString(b)
}
