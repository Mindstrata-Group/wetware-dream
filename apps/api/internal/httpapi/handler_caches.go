package httpapi

import (
	"encoding/json"
	"sync"
	"time"
)

type authRateLimiterCache struct {
	sync.Mutex
	items     map[string]authRateWindow
	lastSweep time.Time
}

type chatRateLimiterCache struct {
	sync.Mutex
	items     map[int64]authRateWindow
	lastSweep time.Time
}

type modesCacheState struct {
	sync.RWMutex
	expiresAt time.Time
	modes     []ModeOption
}

type publicDemoModesCacheState struct {
	sync.RWMutex
	expiresAt time.Time
	modes     []DemoModeOption
}

type siteContentCacheState struct {
	sync.RWMutex
	expiresAt time.Time
	data      map[string]json.RawMessage
}

type adminDialogsCacheState struct {
	sync.Mutex
	expiresAt time.Time
	body      []byte
}

func (c *adminDialogsCacheState) clear() {
	c.Lock()
	c.expiresAt = time.Time{}
	c.body = nil
	c.Unlock()
}

type adminMessagesTotalCacheState struct {
	sync.Mutex
	expiresAt time.Time
	value     int64
}

type adminStatsCacheState struct {
	sync.Mutex
	expiresAt time.Time
	payload   map[string]any
}

type cachedKeyBody struct {
	expiresAt time.Time
	body      []byte
}

type adminKeyedCache struct {
	sync.Mutex
	items map[string]cachedKeyBody
}

func (c *adminKeyedCache) get(key string) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()
	item, ok := c.items[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.body, true
}

func (c *adminKeyedCache) set(key string, body []byte, ttl time.Duration) {
	c.Lock()
	c.items[key] = cachedKeyBody{expiresAt: time.Now().Add(ttl), body: body}
	c.Unlock()
}

func (c *adminKeyedCache) clear() {
	c.Lock()
	c.items = map[string]cachedKeyBody{}
	c.Unlock()
}

// handlerCaches holds all of the Handler's in-memory caches.
// They were moved out of package-global variables for isolation between tests
// (each NewTestServer creates its own instance, so tests can safely run in parallel).
type handlerCaches struct {
	authRL                 authRateLimiterCache
	chatRL                 chatRateLimiterCache
	modes                  modesCacheState
	publicDemoModes        publicDemoModesCacheState
	siteContent            siteContentCacheState
	adminDialogs           adminDialogsCacheState
	adminMessagesTotal     adminMessagesTotalCacheState
	adminStats             adminStatsCacheState
	adminModes             adminKeyedCache
	adminPromoList         adminKeyedCache
	adminPromoAdmin        adminKeyedCache
	adminUsersList         adminKeyedCache
	adminAISettings        simpleGlobalCache
	adminAnalyticsSettings simpleGlobalCache
	publicAnalytics        simpleGlobalCache
	adminTariffGroups      simpleGlobalCache
	adminSummaryPrompts    simpleGlobalCache
	adminOrchestration     simpleGlobalCache
	adminDialogSummary     simpleGlobalCache
	adminLeadSummaryPrompt simpleGlobalCache
	adminModeModelStats    simpleGlobalCache
	adminAIModels          adminKeyedCache
	vertexToken            vertexTokenCache
	aiGateways             aiGatewaysCache
}

// vertexTokenCache is the hourly service account OAuth token for Vertex AI.
// It lives in the cache rather than being requested on every call: the token is valid for an hour, and
// we have a handful of chats per day, so fetching a new one each time would triple
// the number of calls to Google for no benefit.
type vertexTokenCache struct {
	sync.RWMutex
	token     string
	expiresAt time.Time
}

func newHandlerCaches() *handlerCaches {
	return &handlerCaches{
		authRL:          authRateLimiterCache{items: map[string]authRateWindow{}},
		chatRL:          chatRateLimiterCache{items: map[int64]authRateWindow{}},
		adminModes:      adminKeyedCache{items: map[string]cachedKeyBody{}},
		adminPromoList:  adminKeyedCache{items: map[string]cachedKeyBody{}},
		adminPromoAdmin: adminKeyedCache{items: map[string]cachedKeyBody{}},
		adminUsersList:  adminKeyedCache{items: map[string]cachedKeyBody{}},
		adminAIModels:   adminKeyedCache{items: map[string]cachedKeyBody{}},
	}
}

// InitHandlerCaches initialises the Handler's caches. Called from main.go.
func InitHandlerCaches(h *Handler) {
	h.c = newHandlerCaches()
}
