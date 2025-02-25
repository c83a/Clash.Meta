package common

import (
	"strconv"

	"github.com/c83a/Clash.Meta/component/geodata"
	"github.com/c83a/Clash.Meta/component/mmdb"
	C "github.com/c83a/Clash.Meta/constant"
	"github.com/c83a/Clash.Meta/log"
)

type ASN struct {
	*Base
	asn         string
	adapter     string
	noResolveIP bool
	isSourceIP  bool
}

func (a *ASN) Match(metadata *C.Metadata) (bool, string) {
	ip := metadata.DstIP
	if a.isSourceIP {
		ip = metadata.SrcIP
	}
	if !ip.IsValid() {
		return false, ""
	}

	result := mmdb.ASNInstance().LookupASN(ip.AsSlice())
	asnNumber := strconv.FormatUint(uint64(result.AutonomousSystemNumber), 10)
	if !a.isSourceIP {
		metadata.DstIPASN = asnNumber + " " + result.AutonomousSystemOrganization
	}

	match := a.asn == asnNumber
	return match, a.adapter
}

func (a *ASN) RuleType() C.RuleType {
	if a.isSourceIP {
		return C.SrcIPASN
	}
	return C.IPASN
}

func (a *ASN) Adapter() string {
	return a.adapter
}

func (a *ASN) Payload() string {
	return a.asn
}

func (a *ASN) ShouldResolveIP() bool {
	return !a.noResolveIP
}

func (a *ASN) GetASN() string {
	return a.asn
}

func NewIPASN(asn string, adapter string, isSrc, noResolveIP bool) (*ASN, error) {
	C.ASNEnable = true
	if err := geodata.InitASN(); err != nil {
		log.Errorln("can't initial ASN: %s", err)
		return nil, err
	}

	return &ASN{
		Base:        &Base{},
		asn:         asn,
		adapter:     adapter,
		noResolveIP: noResolveIP,
		isSourceIP:  isSrc,
	}, nil
}
