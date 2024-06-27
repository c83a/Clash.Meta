package outboundgroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
	"runtime"

	"github.com/c83a/Clash.Meta/adapter/outbound"
	"github.com/c83a/Clash.Meta/common/callback"
	"github.com/c83a/Clash.Meta/common/lru"
	N "github.com/c83a/Clash.Meta/common/net"
	"github.com/c83a/Clash.Meta/common/utils"
	"github.com/c83a/Clash.Meta/component/dialer"
	C "github.com/c83a/Clash.Meta/constant"
	"github.com/c83a/Clash.Meta/constant/provider"
	"github.com/c83a/Clash.Meta/common/atomic"

	"golang.org/x/net/publicsuffix"
)

type strategyFn = func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy

type LoadBalance struct {
	*GroupBase
	disableUDP     bool
	strategyFn     strategyFn
	chAlive        chan []C.Proxy
	testUrl        string
	expectedStatus string
	Hidden         bool
	Icon           string
}

var errStrategy = errors.New("unsupported strategy")

func parseStrategy(config map[string]any) string {
	if strategy, ok := config["strategy"].(string); ok {
		return strategy
	}
	return "consistent-hashing"
}

func getKey(metadata *C.Metadata) string {
	if metadata == nil {
		return ""
	}

	if metadata.Host != "" {
		// ip host
		if ip := net.ParseIP(metadata.Host); ip != nil {
			return metadata.Host
		}

		if etld, err := publicsuffix.EffectiveTLDPlusOne(metadata.Host); err == nil {
			return etld
		}
	}

	if !metadata.DstIP.IsValid() {
		return ""
	}

	return metadata.DstIP.String()
}

func getKeyWithSrcAndDst(metadata *C.Metadata) string {
	dst := getKey(metadata)
	src := ""
	if metadata != nil {
		src = metadata.SrcIP.String()
	}

	return fmt.Sprintf("%s%s", src, dst)
}

func jumpHash(key uint64, buckets int32) int32 {
	var b, j int64

	for j < int64(buckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	return int32(b)
}

// DialContext implements C.ProxyAdapter
func (lb *LoadBalance) DialContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (c C.Conn, err error) {
	proxy := lb.Unwrap(metadata, true)
	c, err = proxy.DialContext(ctx, metadata, lb.Base.DialOptions(opts...)...)

	if err == nil {
		c.AppendToChains(lb)
	} else {
		lb.onDialFailed(proxy.Type(), err)
	}

	if N.NeedHandshake(c) {
		c = callback.NewFirstWriteCallBackConn(c, func(err error) {
			if err == nil {
				lb.onDialSuccess()
			} else {
				lb.onDialFailed(proxy.Type(), err)
			}
		})
	}

	return
}

// ListenPacketContext implements C.ProxyAdapter
func (lb *LoadBalance) ListenPacketContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (pc C.PacketConn, err error) {
	defer func() {
		if err == nil {
			pc.AppendToChains(lb)
		}
	}()

	proxy := lb.Unwrap(metadata, true)
	return proxy.ListenPacketContext(ctx, metadata, lb.Base.DialOptions(opts...)...)
}

// SupportUDP implements C.ProxyAdapter
func (lb *LoadBalance) SupportUDP() bool {
	return !lb.disableUDP
}

// IsL3Protocol implements C.ProxyAdapter
func (lb *LoadBalance) IsL3Protocol(metadata *C.Metadata) bool {
	return lb.Unwrap(metadata, false).IsL3Protocol(metadata)
}

func (lb *LoadBalance) Hint() {
	if lb.chAlive != nil{
		return
	}
	var proxies_alive []C.Proxy
	var pxs []C.Proxy
	url := lb.testUrl
	chHints := lb.chHints
	chDone := make(chan struct{})
	chAlive := make(chan []C.Proxy)
	lb.chAlive = chAlive
	pxs = lb.GetProxies(false)
	for _, py := range(pxs){
		if !py.AliveForTestUrl(url){
			continue}
		proxies_alive = append(proxies_alive, py)}
	if len(proxies_alive) == 0{
		proxies_alive = pxs
	}
	pxs = nil
	runtime.SetFinalizer(lb, func(x any){chDone <- struct{}{}})
	for{
		select{
			case <- chDone:
				return
			case <- chHints:
				proxies_alive = make([]C.Proxy,0,4)
				pxs = lb.GetProxies(false)
				for _, py := range(pxs){
				if !py.AliveForTestUrl(url) {continue}
				proxies_alive = append(proxies_alive, py)}
				if len(proxies_alive) == 0{
					proxies_alive = pxs }
				pxs = nil
			case chAlive <- proxies_alive:
		}


	}
}

func strategyRoundRobin(lb *LoadBalance, url string) strategyFn {
	var i atomic.Int64 = atomic.NewInt64(-1)
	return func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy {
		if lb.chAlive != nil{
			pxs := <- lb.chAlive
			return pxs[int(i.Add(1)) % len(pxs)]
		}
		return lb.Unwrap( nil, false)

}
}
func strategyConsistentHashing(lb *LoadBalance, url string) strategyFn {
	maxRetry := 5
	return func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy {
		key := utils.MapHash(getKey(metadata))
		buckets := int32(len(proxies))
		for i := 0; i < maxRetry; i, key = i+1, key+1 {
			idx := jumpHash(key, buckets)
			proxy := proxies[idx]
			if proxy.AliveForTestUrl(url) {
				return proxy
			}
		}

		// when availability is poor, traverse the entire list to get the available nodes
		for _, proxy := range proxies {
			if proxy.AliveForTestUrl(url) {
				return proxy
			}
		}

		return proxies[0]
	}
}

func strategyStickySessions(lb *LoadBalance, url string) strategyFn {
	ttl := time.Minute * 10
	maxRetry := 5
	lruCache := lru.New[uint64, int](
		lru.WithAge[uint64, int](int64(ttl.Seconds())),
		lru.WithSize[uint64, int](1000))
	return func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy {
		key := utils.MapHash(getKeyWithSrcAndDst(metadata))
		length := len(proxies)
		idx, has := lruCache.Get(key)
		if !has {
			idx = int(jumpHash(key+uint64(time.Now().UnixNano()), int32(length)))
		}

		nowIdx := idx
		for i := 1; i < maxRetry; i++ {
			proxy := proxies[nowIdx]
			if proxy.AliveForTestUrl(url) {
				if nowIdx != idx {
					lruCache.Delete(key)
					lruCache.Set(key, nowIdx)
				}

				return proxy
			} else {
				nowIdx = int(jumpHash(key+uint64(time.Now().UnixNano()), int32(length)))
			}
		}

		lruCache.Delete(key)
		lruCache.Set(key, 0)
		return proxies[0]
	}
}

// Unwrap implements C.ProxyAdapter
func (lb *LoadBalance) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	proxies := lb.GetProxies(touch)
	if lb.chAlive != nil{
		return lb.strategyFn(proxies, metadata, touch)}
	lb.locker.Lock()
	defer lb.locker.Unlock()
	if lb.chAlive != nil{
		return lb.strategyFn(proxies, metadata, touch)}
	go lb.Hint()
	runtime.Gosched()
	return lb.strategyFn(proxies, metadata, touch)

}

// MarshalJSON implements C.ProxyAdapter
func (lb *LoadBalance) MarshalJSON() ([]byte, error) {
	var all []string
	for _, proxy := range lb.GetProxies(false) {
		all = append(all, proxy.Name())
	}
	return json.Marshal(map[string]any{
		"type":           lb.Type().String(),
		"all":            all,
		"testUrl":        lb.testUrl,
		"expectedStatus": lb.expectedStatus,
		"hidden":         lb.Hidden,
		"icon":           lb.Icon,
	})
}

func NewLoadBalance(option *GroupCommonOption, providers []provider.ProxyProvider, strategy string) (lb *LoadBalance, err error) {
	l := &LoadBalance{
		GroupBase: NewGroupBase(GroupBaseOption{
			outbound.BaseOption{
				Name:        option.Name,
				Type:        C.LoadBalance,
				Interface:   option.Interface,
				RoutingMark: option.RoutingMark,
			},
			option.Filter,
			option.ExcludeFilter,
			option.ExcludeType,
			option.TestTimeout,
			option.MaxFailedTimes,
			providers,
		}),
		disableUDP:     option.DisableUDP,
		testUrl:        option.URL,
		expectedStatus: option.ExpectedStatus,
		Hidden:         option.Hidden,
		Icon:           option.Icon,
	}
	var strategyFn strategyFn
	switch strategy {
	case "consistent-hashing":
		strategyFn = strategyConsistentHashing(l, option.URL)
	case "round-robin":
		strategyFn = strategyRoundRobin(l, option.URL)
	case "sticky-sessions":
		strategyFn = strategyStickySessions(l, option.URL)
	default:
		return nil, fmt.Errorf("%w: %s", errStrategy, strategy)
	}
	l.strategyFn=strategyFn
	return l, nil
}
