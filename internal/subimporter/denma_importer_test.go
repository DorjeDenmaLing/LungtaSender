package subimporter

import (
	"sync"
	"testing"
	"time"
)

// isDone answers while a writer waits for the lock.
func TestDenmaIsDoneUnderWaitingWriter(t *testing.T) {
	im := &Importer{stop: make(chan bool, 1), status: Status{Status: StatusImporting}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < 200; i++ {
			wg.Add(2)
			go func() { defer wg.Done(); im.setStatus(StatusImporting) }()
			go func() { defer wg.Done(); im.isDone() }()
		}
		wg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("isDone deadlocked with a writer waiting")
	}
}

// Two sessions started at once: only one starts.
func TestDenmaOneSessionAtATime(t *testing.T) {
	im := &Importer{stop: make(chan bool, 1)}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
		ok int
	)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := im.NewSession(SessionOpt{Filename: "x.csv", Mode: ModeSubscribe}); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d sessions started at once, want 1", ok)
	}
}

// A stop left over from an earlier import doesn't stop the next session.
func TestDenmaLeftoverStopIgnored(t *testing.T) {
	im := &Importer{stop: make(chan bool, 1)}
	im.stop <- true
	if _, err := im.NewSession(SessionOpt{Filename: "x.csv", Mode: ModeSubscribe}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-im.stop:
		t.Fatal("a stop from before the session is still waiting")
	default:
	}
}
