package rdap

import (
	"context"
	"sync"
	"testing"
	"time"
)

// A warm cache hit used to read bootstrapMap after dropping the lock, which
// races with the refresh store. Run with -race.
func TestBootstrapCacheHitIsSynchronised(t *testing.T) {
	warm := map[string][]string{"com": {"https://rdap.verisign.com/com/v1"}}

	bootstrapMu.Lock()
	bootstrapMap, bootstrapAt = warm, time.Now()
	bootstrapMu.Unlock()

	t.Cleanup(func() {
		bootstrapMu.Lock()
		bootstrapMap, bootstrapAt = nil, time.Time{}
		bootstrapMu.Unlock()
	})

	var wg sync.WaitGroup
	// the refresh store, exactly as the cold path publishes it
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			bootstrapMu.Lock()
			bootstrapMap, bootstrapAt = warm, time.Now()
			bootstrapMu.Unlock()
		}
	}()

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				// stays on the cache-hit path, so no network is touched
				if mapping, err := Bootstrap(context.Background()); err != nil || mapping["com"] == nil {
					t.Errorf("cache hit returned %v, %v", mapping, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
