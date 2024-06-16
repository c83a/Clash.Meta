package outboundgroup

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"

	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/component/dialer"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/constant/provider"
)

type Selector struct {
	*GroupBase
	disableUDP bool
	hint       *C.Proxy
	selected   string
	Hidden     bool
	Icon       string
}

// DialContext implements C.ProxyAdapter
func (s *Selector) DialContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (C.Conn, error) {
	c, err := s.selectedProxy(true).DialContext(ctx, metadata, s.Base.DialOptions(opts...)...)
	if err == nil {
		c.AppendToChains(s)
	}
	return c, err
}

// ListenPacketContext implements C.ProxyAdapter
func (s *Selector) ListenPacketContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (C.PacketConn, error) {
	pc, err := s.selectedProxy(true).ListenPacketContext(ctx, metadata, s.Base.DialOptions(opts...)...)
	if err == nil {
		pc.AppendToChains(s)
	}
	return pc, err
}

// SupportUDP implements C.ProxyAdapter
func (s *Selector) SupportUDP() bool {
	if s.disableUDP {
		return false
	}

	return s.selectedProxy(false).SupportUDP()
}

// IsL3Protocol implements C.ProxyAdapter
func (s *Selector) IsL3Protocol(metadata *C.Metadata) bool {
	return s.selectedProxy(false).IsL3Protocol(metadata)
}

// MarshalJSON implements C.ProxyAdapter
func (s *Selector) MarshalJSON() ([]byte, error) {
	all := []string{}
	for _, proxy := range s.GetProxies(false) {
		all = append(all, proxy.Name())
	}

	return json.Marshal(map[string]any{
		"type":   s.Type().String(),
		"now":    s.Now(),
		"all":    all,
		"hidden": s.Hidden,
		"icon":   s.Icon,
	})
}

func (s *Selector) Now() string {
	return s.selectedProxy(false).Name()
}

func (s *Selector) Set(name string) error {
	p := new(C.Proxy)
	for _, proxy := range s.GetProxies(false) {
		if proxy.Name() == name {
			*p = proxy
			s.hint = p
			s.selected = name
			return nil
		}
	}

	return errors.New("proxy not exist")
}

func (s *Selector) ForceSet(name string) {
	s.selected = name
}

// Unwrap implements C.ProxyAdapter
func (s *Selector) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	return s.selectedProxy(touch)
}

func (s *Selector) selectedProxy(touch bool) C.Proxy {
	var pp *C.Proxy
	pp = s.hint
	if pp != nil{
		return *pp}
	s.locker.Lock()
	defer s.locker.Unlock()
	chDone := make(chan struct{})
	go s.Hint(chDone)
	runtime.SetFinalizer(s, func(x any){chDone <-struct{}{}})
	pp = s.hint
	if pp != nil{
		return *pp}
	return s._selectedProxy()
}
func (s *Selector) Hint(chDone <-chan struct{}){
	chHints := s.GetHints()
	s._selectedProxy()
	for{select {
	case <-chHints:
		s._selectedProxy()
	case <-chDone:
		return
	}}
}
func (s *Selector) _selectedProxy() C.Proxy {
	proxies := s.GetProxies(false)
	p := new(C.Proxy)
	for _, proxy := range proxies {
		if proxy.Name() == s.selected {
			*p = proxy
			s.hint = p
			return proxy
		}
	}

	*p = proxies[0]
	s.selected = (*p).Name()
	s.hint=p
	return *p
}

func NewSelector(option *GroupCommonOption, providers []provider.ProxyProvider) *Selector {

	return &Selector{
		GroupBase: NewGroupBase(GroupBaseOption{
			outbound.BaseOption{
				Name:        option.Name,
				Type:        C.Selector,
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
		selected:   "COMPATIBLE",
		disableUDP: option.DisableUDP,
		Hidden:     option.Hidden,
		Icon:       option.Icon,
	}
}
