package outboundgroup

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"runtime"

	"github.com/c83a/Clash.Meta/adapter/outbound"
	"github.com/c83a/Clash.Meta/common/atomic"
	"github.com/c83a/Clash.Meta/common/utils"
	C "github.com/c83a/Clash.Meta/constant"
	"github.com/c83a/Clash.Meta/constant/provider"
	types "github.com/c83a/Clash.Meta/constant/provider"
	"github.com/c83a/Clash.Meta/log"
	"github.com/c83a/Clash.Meta/tunnel"

	"github.com/dlclark/regexp2"
)

type GroupBase struct {
	*outbound.Base
	filterRegs       []*regexp2.Regexp
	excludeFilterReg *regexp2.Regexp
	excludeTypeArray []string
	providers        []provider.ProxyProvider
	failedTestMux    sync.Mutex
	failedTimes      int
	failedTime       time.Time
	failedTesting    atomic.Bool
	proxies          [][]C.Proxy
	ChProxies        chan []C.Proxy
	chHints          chan struct{}
	locker           sync.Mutex
	versions         []atomic.Uint32
	TestTimeout      int
	maxFailedTimes   int
}

type GroupBaseOption struct {
	outbound.BaseOption
	filter         string
	excludeFilter  string
	excludeType    string
	TestTimeout    int
	maxFailedTimes int
	providers      []provider.ProxyProvider
}

func NewGroupBase(opt GroupBaseOption) *GroupBase {
	var excludeFilterReg *regexp2.Regexp
	if opt.excludeFilter != "" {
		excludeFilterReg = regexp2.MustCompile(opt.excludeFilter, regexp2.None)
	}
	var excludeTypeArray []string
	if opt.excludeType != "" {
		excludeTypeArray = strings.Split(opt.excludeType, "|")
	}

	var filterRegs []*regexp2.Regexp
	if opt.filter != "" {
		for _, filter := range strings.Split(opt.filter, "`") {
			filterReg := regexp2.MustCompile(filter, regexp2.None)
			filterRegs = append(filterRegs, filterReg)
		}
	}

	gb := &GroupBase{
		Base:             outbound.NewBase(opt.BaseOption),
		filterRegs:       filterRegs,
		excludeFilterReg: excludeFilterReg,
		excludeTypeArray: excludeTypeArray,
		providers:        opt.providers,
		failedTesting:    atomic.NewBool(false),
		TestTimeout:      opt.TestTimeout,
		maxFailedTimes:   opt.maxFailedTimes,
	}

	if gb.TestTimeout == 0 {
		gb.TestTimeout = 5000
	}
	if gb.maxFailedTimes == 0 {
		gb.maxFailedTimes = 5
	}

	gb.proxies = make([][]C.Proxy, len(opt.providers))
	gb.versions = make([]atomic.Uint32, len(opt.providers))

	return gb
}

func (gb *GroupBase) Touch() {
	for _, pd := range gb.providers {
		pd.Touch()
	}
}

func (gb *GroupBase) GetProxies(touch bool)([]C.Proxy ){
	if gb.ChProxies != nil {
		return  <- gb.ChProxies
	}
	return  gb.GetProxiesCH()
}

func (gb *GroupBase) GetProxiesCH()( []C.Proxy ){
	if gb.ChProxies != nil {
		return  <- gb.ChProxies
	}
	gb.locker.Lock()
	defer gb.locker.Unlock()

	if gb.ChProxies != nil {
		return  <- gb.ChProxies
	}
	chProxies := make(chan []C.Proxy)
	chDirtyCache := make(chan struct{},1)
	chHints := make(chan struct{},1)
	chDone := make(chan struct{})
	for _, pd := range gb.providers {
		pd.AddFollower(chDirtyCache)
	}
	gb.ChProxies = chProxies
	gb.chHints = chHints
	runtime.SetFinalizer(gb,func(x any){chDone <- struct{}{}})
	cached_proxies := gb._GetProxies(make([]C.Proxy,0,2))
	go func(){var hint struct{}
		for {
		select{
		case hint = <- chDirtyCache:
			cached_proxies = gb._GetProxies(cached_proxies[:0])
			select {
			case chHints <- hint:
			default:
			}
		case chProxies <- cached_proxies:
			//pass
		case <- chDone:
			return
		}}
	}()
	return  cached_proxies
}

func (gb *GroupBase) GetHints() chan struct{}{
	return gb.chHints
}
func (gb *GroupBase) _GetProxies(proxies []C.Proxy) []C.Proxy {
//	var proxies []C.Proxy
	for count:=0;count < 2; count +=1 {
	runtime.Gosched()
	if len(gb.filterRegs) == 0 {
		for _, pd := range gb.providers {
/*			if touch {
				pd.Touch()
			}
*/
			proxies = append(proxies, pd.Proxies()...)
		}
	} else {
		for i, pd := range gb.providers {
/*			if touch {
				pd.Touch()
			}
*/
			if pd.VehicleType() == types.Compatible {
				gb.versions[i].Store(pd.Version())
				gb.proxies[i] = pd.Proxies()
				continue
			}

			version := gb.versions[i].Load()
			if version != pd.Version() && gb.versions[i].CompareAndSwap(version, pd.Version()) {
				var (
					proxies    []C.Proxy
					newProxies []C.Proxy
				)

				proxies = pd.Proxies()
				proxiesSet := map[string]struct{}{}
				for _, filterReg := range gb.filterRegs {
					for _, p := range proxies {
						name := p.Name()
						if mat, _ := filterReg.MatchString(name); mat {
							if _, ok := proxiesSet[name]; !ok {
								proxiesSet[name] = struct{}{}
								newProxies = append(newProxies, p)
							}
						}
					}
				}

				gb.proxies[i] = newProxies
			}
		}

		for _, p := range gb.proxies {
			proxies = append(proxies, p...)
		}
	}

	if len(gb.providers) > 1 && len(gb.filterRegs) > 1 {
		var newProxies []C.Proxy
		proxiesSet := map[string]struct{}{}
		for _, filterReg := range gb.filterRegs {
			for _, p := range proxies {
				name := p.Name()
				if mat, _ := filterReg.MatchString(name); mat {
					if _, ok := proxiesSet[name]; !ok {
						proxiesSet[name] = struct{}{}
						newProxies = append(newProxies, p)
					}
				}
			}
		}
		for _, p := range proxies { // add not matched proxies at the end
			name := p.Name()
			if _, ok := proxiesSet[name]; !ok {
				proxiesSet[name] = struct{}{}
				newProxies = append(newProxies, p)
			}
		}
		proxies = newProxies
	}
	if gb.excludeTypeArray != nil {
		var newProxies []C.Proxy
		for _, p := range proxies {
			mType := p.Type().String()
			flag := false
			for i := range gb.excludeTypeArray {
				if strings.EqualFold(mType, gb.excludeTypeArray[i]) {
					flag = true
					break
				}

			}
			if flag {
				continue
			}
			newProxies = append(newProxies, p)
		}
		proxies = newProxies
	}

	if gb.excludeFilterReg != nil {
		var newProxies []C.Proxy
		for _, p := range proxies {
			name := p.Name()
			if mat, _ := gb.excludeFilterReg.MatchString(name); mat {
				continue
			}
			newProxies = append(newProxies, p)
		}
		proxies = newProxies
	}
	if len(proxies) != 0 {
		break
	}
}
	if len(proxies) == 0 {
		return append(proxies, tunnel.Proxies()["COMPATIBLE"])
	}

	return proxies
}

func (gb *GroupBase) URLTest(ctx context.Context, url string, expectedStatus utils.IntRanges[uint16]) (map[string]uint16, error) {
	var wg sync.WaitGroup
	var lock sync.Mutex
	mp := map[string]uint16{}
	proxies := gb.GetProxies(false)
	for _, proxy := range proxies {
		proxy := proxy
		wg.Add(1)
		go func() {
			delay, err := proxy.URLTest(ctx, url, expectedStatus)
			if err == nil {
				lock.Lock()
				mp[proxy.Name()] = delay
				lock.Unlock()
			}

			wg.Done()
		}()
	}
	wg.Wait()
	select {case gb.chHints <- struct{}{}:
	default:}

	if len(mp)==0{
		return mp, fmt.Errorf("get delay: all proxies timeout")
	}
	return mp, nil
}

func (gb *GroupBase) onDialFailed(adapterType C.AdapterType, err error) {
	if adapterType == C.Direct || adapterType == C.Compatible || adapterType == C.Reject || adapterType == C.Pass || adapterType == C.RejectDrop {
		return
	}

	if strings.Contains(err.Error(), "connection refused") {
		go gb.healthCheck()
		return
	}

	go func() {
		gb.failedTestMux.Lock()
		defer gb.failedTestMux.Unlock()

		gb.failedTimes++
		if gb.failedTimes == 1 {
			log.Debugln("ProxyGroup: %s first failed", gb.Name())
			gb.failedTime = time.Now()
		} else {
			if time.Since(gb.failedTime) > time.Duration(gb.TestTimeout)*time.Millisecond {
				gb.failedTimes = 0
				return
			}

			log.Debugln("ProxyGroup: %s failed count: %d", gb.Name(), gb.failedTimes)
			if gb.failedTimes >= gb.maxFailedTimes {
				log.Warnln("because %s failed multiple times, active health check", gb.Name())
				gb.healthCheck()
			}
		}
	}()
}

func (gb *GroupBase) healthCheck() {
	if gb.failedTesting.Load() {
		return
	}

	gb.failedTesting.Store(true)
	wg := sync.WaitGroup{}
	for _, proxyProvider := range gb.providers {
		wg.Add(1)
		proxyProvider := proxyProvider
		go func() {
			defer wg.Done()
			proxyProvider.HealthCheck()
		}()
	}

	wg.Wait()
	gb.failedTesting.Store(false)
	gb.failedTimes = 0
	select {case gb.chHints <- struct{}{}:
	default:}
}

func (gb *GroupBase) onDialSuccess() {
	if !gb.failedTesting.Load() {
		gb.failedTimes = 0
	}
}
