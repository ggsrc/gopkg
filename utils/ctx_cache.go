package utils

import (
	"context"
	"sync"
	"sync/atomic"
)

type cacheItem struct {
	ret  interface{}
	err  error
	once sync.Once
	done uint32 // 付出一个atomic的代价，避免侵入sync.Once
}

func newCacheItem() *cacheItem {
	return &cacheItem{
		once: sync.Once{},
	}
}

func (ci *cacheItem) doOnce(ctx context.Context, loader func(context.Context) (interface{}, error)) {
	ci.once.Do(func() {
		defer atomic.StoreUint32(&ci.done, 1)
		// sync.Once guarantees that only one routine will execute this, the others will wait till return
		ci.ret, ci.err = loader(ctx)
	})
}

type callCache struct {
	m    map[string]*cacheItem // sync.Map的LoadOrStore方法的参数会逃逸到heap上，这里用map+rwmutex
	lock sync.RWMutex
}

// getOrCreateCacheItem 从callCache中获取指定key的cacheItem(不存在则创建一个)。保证并发安全
// 不会返回nil
func (cache *callCache) getOrCreateCacheItem(key string) *cacheItem {
	cache.lock.RLock()
	cr, ok := cache.m[key]
	cache.lock.RUnlock()
	if ok {
		return cr
	}

	cache.lock.Lock()
	defer cache.lock.Unlock()
	if cache.m == nil {
		cache.m = make(map[string]*cacheItem)
	} else {
		cr, ok = cache.m[key]
	}
	if !ok {
		cr = newCacheItem()
		cache.m[key] = cr
	}
	return cr
}

const callCacheKey string = "_g_call_cache"
const singleflight string = "_g_singleflight"
const memCacheKey string = "_g_mem_cache"

// WithCallCache 返回支持调用缓存的context
func WithCallCache(parent context.Context) context.Context {
	if parent.Value(callCacheKey) != nil {
		return parent
	}
	return context.WithValue(parent, callCacheKey, new(callCache)) // nolint: staticcheck
}

// WithSingleflight 返回支持调用singleflight的context
func WithSingleflight(parent context.Context) context.Context {
	if parent.Value(singleflight) != nil {
		return parent
	}
	return context.WithValue(parent, singleflight, new(callCache)) // nolint: staticcheck
}

// WithMemCache 返回支持memory cache的context
func WithMemCache(parent context.Context) context.Context {
	if parent.Value(memCacheKey) != nil {
		return parent
	}
	return context.WithValue(parent, memCacheKey, new(callCache)) // nolint: staticcheck
}

type loadFunc[T any] func(context.Context) (T, error)

func ContextCacheExists(ctx context.Context) bool {
	if v := ctx.Value(callCacheKey); v != nil {
		return true
	}
	return false
}

func SingleflightEnable(ctx context.Context) bool {
	if v := ctx.Value(singleflight); v != nil {
		return true
	}
	return false
}

func MemCacheEnable(ctx context.Context) bool {
	if v := ctx.Value(memCacheKey); v != nil {
		return true
	}
	return false
}

// getOrCreateCacheItem 未启用cache才会返回nil
func getOrCreateCacheItem(ctx context.Context, key string) *cacheItem {
	if v := ctx.Value(callCacheKey); v != nil {
		return v.(*callCache).getOrCreateCacheItem(key)
	}
	return nil
}

// LoadFromCtxCache 从ctx中尝试获取key的缓存结果
// 如果不存在，调用loader;如果没有开启缓存，直接调用loader
func LoadFromCtxCache[T any](ctx context.Context, key string, loader loadFunc[T]) (T, error) {
	myCacheItem := getOrCreateCacheItem(ctx, key)
	if myCacheItem == nil { // cache not enabled
		return loader(ctx)
	}

	// Wrapper function to convert the loader to the required type
	loaderWrapper := func(ctx context.Context) (interface{}, error) {
		return loader(ctx)
	}

	// now that all routines hold references to the same cacheItem
	myCacheItem.doOnce(ctx, loaderWrapper)
	if myCacheItem.err != nil {
		var zero T
		return zero, myCacheItem.err
	}
	return myCacheItem.ret.(T), myCacheItem.err
}

// TryLoadFromCtxCache 从ctx中尝试获取key的缓存结果
// 如果不存在或loader正在执行中，返回false和空结果。如果存在load好的缓存，返回true和缓存
func TryLoadFromCtxCache(ctx context.Context, key string) (bool, interface{}, error) {
	myCacheItem := getOrCreateCacheItem(ctx, key)
	// cache not enabled or not done
	if myCacheItem == nil || atomic.LoadUint32(&myCacheItem.done) == 0 {
		return false, nil, nil
	}
	return true, myCacheItem.ret, myCacheItem.err
}
