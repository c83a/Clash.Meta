package common

import (
	"github.com/c83a/Clash.Meta/component/ipset"
	C "github.com/c83a/Clash.Meta/constant"
	"github.com/c83a/Clash.Meta/log"
)

type IPSet struct {
	*Base
	name        string
	adapter     string
	noResolveIP bool
}

func (f *IPSet) RuleType() C.RuleType {
	return C.IPSet
}

func (f *IPSet) Match(metadata *C.Metadata) (bool, string){
	exist, err := ipset.Test(f.name, metadata.DstIP)
	if err != nil {
		log.Warnln("check ipset '%s' failed: %s", f.name, err.Error())
		return false,f.adapter
	}
	return exist,f.adapter
}

func (f *IPSet) Adapter() string {
	return f.adapter
}

func (f *IPSet) Payload() string {
	return f.name
}

func (f *IPSet) ShouldResolveIP() bool {
	return !f.noResolveIP
}

func (f *IPSet) ShouldFindProcess() bool {
	return false
}

func NewIPSet(name string, adapter string, noResolveIP bool) (*IPSet, error) {
	if err := ipset.Verify(name); err != nil {
		return nil, err
	}

	return &IPSet{
		Base:    &Base{},
		name:        name,
		adapter:     adapter,
		noResolveIP: noResolveIP,
	}, nil
}
