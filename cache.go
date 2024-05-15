package requests_cache

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	statusInProgress uint32 = 1
	statusDone       uint32 = 0
)

var (
	maxCacheSize = 1000
	cacheTTL     = 5 * time.Second
)

var once sync.Once

var rc *requestCache

type cache struct {
	tStart         time.Time
	ctx            context.Context
	progressStatus *uint32
	count          *uint32
	waited         *uint32

	data *http.Response
	err  error
}

func (c *cache) inProgress() bool {
	return statusInProgress == atomic.LoadUint32(c.progressStatus)
}

type requestCache struct {
	m     sync.RWMutex
	cache map[string]*cache
}

func (r *requestCache) do(req *http.Request, client http.Client) (*http.Response, error) {
	var val *cache
	var ok bool
	r.m.RLock()
	val, ok = r.cache[req.URL.String()]
	r.m.RUnlock()

	if ok && !val.inProgress() {
		return val.data, val.err
	}

	if !ok {
		ctx, cancel := context.WithCancel(context.Background())
		progressStatus := statusInProgress
		var count uint32 = 1
		var waited uint32 = 1

		r.m.Lock()
		r.cache[req.URL.String()] = &cache{
			ctx:            ctx,
			progressStatus: &progressStatus,
			count:          &count,
			waited:         &waited,
		}
		r.m.Unlock()

		response, err := client.Do(req)

		newProgressStatus := statusDone
		r.m.Lock()
		r.cache[req.URL.String()] = &cache{
			tStart:         time.Now(),
			progressStatus: &newProgressStatus,
			data:           response,
			err:            err,
		}
		r.m.Unlock()

		cancel()
		return response, err
	}

	<-val.ctx.Done()
	val, ok = r.cache[req.URL.String()]
	if ok {
		return val.data, val.err
	}

	return nil, fmt.Errorf("fatal logical error")
}
