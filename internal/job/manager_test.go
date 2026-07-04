package job

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

// waitDone polls until the job leaves the running state.
func waitDone(t *testing.T, m *Manager, id string) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err := m.Get(id)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if st.State != StateRunning {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish: %+v", st)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestJobCountsProgress(t *testing.T) {
	m := NewManager(2)
	var items []Item
	for i := 1; i <= 10; i++ {
		id := i
		items = append(items, Item{LineID: id, Run: func(ctx context.Context) (bool, error) {
			return id%2 == 0, nil // half "cached"
		}})
	}
	jobID := m.Submit(context.Background(), "/out", items)
	st := waitDone(t, m, jobID)

	if st.State != StateDone || st.Total != 10 || st.Done != 10 || st.Cached != 5 || st.Failed != 0 {
		t.Errorf("status: %+v", st)
	}
	if st.OutputDir != "/out" || st.StartedAt == "" || st.FinishedAt == "" {
		t.Errorf("metadata: %+v", st)
	}
}

func TestJobCollectsStructuredFailures(t *testing.T) {
	m := NewManager(1)
	items := []Item{
		{LineID: 1, Run: func(ctx context.Context) (bool, error) { return false, nil }},
		{LineID: 2, Run: func(ctx context.Context) (bool, error) {
			return false, toolerr.New(toolerr.CodeEngineRequest, "HTTP 422")
		}},
		{LineID: 3, Run: func(ctx context.Context) (bool, error) {
			return false, errors.New("plain failure")
		}},
	}
	st := waitDone(t, m, m.Submit(context.Background(), "", items))

	if st.State != StateDone || st.Done != 1 || st.Failed != 2 {
		t.Errorf("status: %+v", st)
	}
	if st.Failures[0].LineID != 2 || st.Failures[0].Code != "engine_request_failed" {
		t.Errorf("failure[0]: %+v", st.Failures[0])
	}
	if st.Failures[1].LineID != 3 || st.Failures[1].Code != "error" {
		t.Errorf("failure[1]: %+v", st.Failures[1])
	}
}

func TestJobRespectsConcurrencyLimit(t *testing.T) {
	m := NewManager(2)
	var cur, peak atomic.Int64
	var mu sync.Mutex
	var items []Item
	for i := 1; i <= 20; i++ {
		items = append(items, Item{LineID: i, Run: func(ctx context.Context) (bool, error) {
			n := cur.Add(1)
			mu.Lock()
			if n > peak.Load() {
				peak.Store(n)
			}
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			cur.Add(-1)
			return false, nil
		}})
	}
	st := waitDone(t, m, m.Submit(context.Background(), "", items))
	if st.Done != 20 {
		t.Errorf("done: %d", st.Done)
	}
	if peak.Load() > 2 {
		t.Errorf("peak concurrency %d exceeds limit 2", peak.Load())
	}
}

func TestJobCancellationMarksFailed(t *testing.T) {
	m := NewManager(1)
	ctx, cancel := context.WithCancel(context.Background())
	var items []Item
	for i := 1; i <= 50; i++ {
		items = append(items, Item{LineID: i, Run: func(ctx context.Context) (bool, error) {
			time.Sleep(10 * time.Millisecond)
			return false, nil
		}})
	}
	jobID := m.Submit(ctx, "", items)
	time.Sleep(30 * time.Millisecond)
	cancel()
	st := waitDone(t, m, jobID)
	if st.State != StateFailed {
		t.Errorf("state: %s", st.State)
	}
	if st.FatalError == "" || st.Done >= 50 {
		t.Errorf("fatal: %q done=%d", st.FatalError, st.Done)
	}
}

func TestGetUnknownJobIsStructured(t *testing.T) {
	m := NewManager(1)
	_, err := m.Get("job_nope")
	var te *toolerr.Error
	if !errors.As(err, &te) || te.Code != toolerr.CodeJobNotFound {
		t.Fatalf("expected job_not_found, got %v", err)
	}
	// The message must guide the agent to recovery.
	if want := "re-run synthesize_script"; !strings.Contains(te.Message, want) {
		t.Errorf("message lacks recovery guidance: %q", te.Message)
	}
}

func TestFailureListTruncation(t *testing.T) {
	m := NewManager(4)
	var items []Item
	for i := 1; i <= 30; i++ {
		items = append(items, Item{LineID: i, Run: func(ctx context.Context) (bool, error) {
			return false, errors.New("boom")
		}})
	}
	st := waitDone(t, m, m.Submit(context.Background(), "", items))
	if st.Failed != 30 || len(st.Failures) != maxReportedFailures || !st.FailuresTruncated {
		t.Errorf("truncation: failed=%d len=%d truncated=%v", st.Failed, len(st.Failures), st.FailuresTruncated)
	}
}

func TestFinishedJobEviction(t *testing.T) {
	m := NewManager(4)
	var ids []string
	for i := 0; i < maxFinishedJobs+5; i++ {
		id := m.Submit(context.Background(), "", []Item{
			{LineID: 1, Run: func(ctx context.Context) (bool, error) { return false, nil }},
		})
		waitDone(t, m, id)
		ids = append(ids, id)
	}
	// Trigger eviction bookkeeping with one more submit.
	last := m.Submit(context.Background(), "", []Item{
		{LineID: 1, Run: func(ctx context.Context) (bool, error) { return false, nil }},
	})
	waitDone(t, m, last)

	evicted := 0
	for _, id := range ids {
		if _, err := m.Get(id); err != nil {
			evicted++
		}
	}
	if evicted == 0 {
		t.Errorf("expected some finished jobs to be evicted")
	}
	if _, err := m.Get(last); err != nil {
		t.Errorf("most recent job must survive: %v", err)
	}
}
