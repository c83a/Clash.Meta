package outboundgroup

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"runtime"

	"github.com/c83a/Clash.Meta/adapter/outbound"
	"github.com/c83a/Clash.Meta/common/callback"
	N "github.com/c83a/Clash.Meta/common/net"
	"github.com/c83a/Clash.Meta/common/utils"
	"github.com/c83a/Clash.Meta/component/dialer"
	C "github.com/c83a/Clash.Meta/constant"
	"github.com/c83a/Clash.Meta/constant/provider"
)

type Fallback struct {
	*GroupBase
	disableUDP     bool
	Hidden         bool
	hint           C.Proxy
	chAlive        chan []C.Proxy
	testUrl        string
	selected       string
	expectedStatus string
	Icon           string
}

func (f *Fallback) Now() string {
	proxy := f.findAliveProxy(false)
	return proxy.Name()
}

// DialContext implements C.ProxyAdapter
func (f *Fallback) DialContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (C.Conn, error) {
	proxy := f.findAliveProxy(true)
	c, err := proxy.DialContext(ctx, metadata, f.Base.DialOptions(opts...)...)
	if err == nil {
		c.AppendToChains(f)
	} else {
		f.onDialFailed(proxy.Type(), err)
	}

	if N.NeedHandshake(c) {
		c = callback.NewFirstWriteCallBackConn(c, func(err error) {
			if err == nil {
				f.onDialSuccess()
			} else {
				f.onDialFailed(proxy.Type(), err)
			}
		})
	}

	return c, err
}

// ListenPacketContext implements C.ProxyAdapter
func (f *Fallback) ListenPacketContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (C.PacketConn, error) {
	proxy := f.findAliveProxy(true)
	pc, err := proxy.ListenPacketContext(ctx, metadata, f.Base.DialOptions(opts...)...)
	if err == nil {
		pc.AppendToChains(f)
	}

	return pc, err
}

// SupportUDP implements C.ProxyAdapter
func (f *Fallback) SupportUDP() bool {
	if f.disableUDP {
		return false
	}

	proxy := f.findAliveProxy(false)
	return proxy.SupportUDP()
}

// IsL3Protocol implements C.ProxyAdapter
func (f *Fallback) IsL3Protocol(metadata *C.Metadata) bool {
	return f.findAliveProxy(false).IsL3Protocol(metadata)
}

// MarshalJSON implements C.ProxyAdapter
func (f *Fallback) MarshalJSON() ([]byte, error) {
	all := []string{}
	for _, proxy := range f.GetProxies(false) {
		all = append(all, proxy.Name())
	}
	return json.Marshal(map[string]any{
		"type":           f.Type().String(),
		"now":            f.Now(),
		"all":            all,
		"testUrl":        f.testUrl,
		"expectedStatus": f.expectedStatus,
		"fixed":          f.selected,
		"hidden":         f.Hidden,
		"icon":           f.Icon,
	})
}

// Unwrap implements C.ProxyAdapter
func (f *Fallback) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	if f.hint != nil{
		return f.hint}
	proxies := f.GetProxies(false)
	f.locker.Lock()
	defer f.locker.Unlock()
	if f.hint != nil{
		return f.hint}
	go f.Hint()
	runtime.Gosched()
	if f.hint != nil{
		return f.hint}
	return f._findAliveProxy(proxies)
}

func (f *Fallback) Hint(){
	if f.hint != nil{
		return
	}
	chDone := make(chan struct{})
	chAlive := make(chan []C.Proxy)
	f.chAlive = chAlive
	chHints := f.chHints
	url := f.testUrl
	runtime.SetFinalizer(f,func(x any){chDone <- struct{}{}})
	var all, alived []C.Proxy
	g :=func(){
		all = f.GetProxies(false)
		for _, p := range all{
			if !p.AliveForTestUrl(url){
				continue
			}
			alived = append(alived, p)
		}
		}
	g()
	all = nil
	go f._findAliveProxy(all)
	for{select{
	case chAlive <- alived:
	case <- chHints:
		alived = nil
		g()
		go f._findAliveProxy(all)
		all = nil
	case <- chDone:
		return
	}}
}
func (f *Fallback) findAliveProxy(touch bool) C.Proxy {
	if f.hint != nil{
		return f.hint}
	return f.Unwrap(nil, false)
}
func (f *Fallback) _findAliveProxy(proxies []C.Proxy) C.Proxy {
	for{
		if f.chAlive != nil{
			break}
		runtime.Gosched()
	}
	alived := <- f.chAlive
	if len(alived) == 0{
		f.hint = proxies[0]
		return f.hint
	}
	for _, p  := range alived{
		if p.Name() == f.selected{
			f.hint = p
			return p
		}
	}
	f.hint = alived[0]
	f.selected = f.hint.Name()
	return f.hint
}

func (f *Fallback) Set(name string) error {
	var p *C.Proxy
	for _, proxy := range f.GetProxies(false) {
		if proxy.Name() == name {
			p = &proxy
			break
		}
	}

	if p == nil {
		return errors.New("proxy not exist")
	}
	f.hint = *p
	f.selected = (*p).Name()
	if !(*p).AliveForTestUrl(f.testUrl) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*time.Duration(5000))
		defer cancel()
		expectedStatus, _ := utils.NewUnsignedRanges[uint16](f.expectedStatus)
		_, _ = (*p).URLTest(ctx, f.testUrl, expectedStatus)
	}

	hints := f.GetHints()
	if hints == nil{
		return nil
	}
	select{
	case hints <- struct{}{}:
	default:
	}
	return nil
}

func (f *Fallback) ForceSet(name string) {
	f.selected = name
}

func NewFallback(option *GroupCommonOption, providers []provider.ProxyProvider) *Fallback {
	return &Fallback{
		GroupBase: NewGroupBase(GroupBaseOption{
			outbound.BaseOption{
				Name:        option.Name,
				Type:        C.Fallback,
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
}
