package common

import (
	"fmt"

	"github.com/c83a/Clash.Meta/component/geodata"
	_ "github.com/c83a/Clash.Meta/component/geodata/memconservative"
	"github.com/c83a/Clash.Meta/component/geodata/router"
	_ "github.com/c83a/Clash.Meta/component/geodata/standard"
	C "github.com/c83a/Clash.Meta/constant"
	"github.com/c83a/Clash.Meta/log"
)

type GEOSITE struct {
	*Base
	country    string
	adapter    string
	matcher    router.DomainMatcher
	recodeSize int
}

func (gs *GEOSITE) RuleType() C.RuleType {
	return C.GEOSITE
}

func (gs *GEOSITE) Match(metadata *C.Metadata) (bool, string) {
	domain := metadata.RuleHost()
	if len(domain) == 0 {
		return false, ""
	}
	return gs.matcher.ApplyDomain(domain), gs.adapter
}

func (gs *GEOSITE) Adapter() string {
	return gs.adapter
}

func (gs *GEOSITE) Payload() string {
	return gs.country
}

func (gs *GEOSITE) GetDomainMatcher() router.DomainMatcher {
	return gs.matcher
}

func (gs *GEOSITE) GetRecodeSize() int {
	return gs.recodeSize
}

func NewGEOSITE(country string, adapter string) (*GEOSITE, error) {
	if err := geodata.InitGeoSite(); err != nil {
		log.Errorln("can't initial GeoSite: %s", err)
		return nil, err
	}

	matcher, size, err := geodata.LoadGeoSiteMatcher(country)
	if err != nil {
		return nil, fmt.Errorf("load GeoSite data error, %s", err.Error())
	}

	log.Infoln("Start initial GeoSite rule %s => %s, records: %d", country, adapter, size)

	geoSite := &GEOSITE{
		Base:       &Base{},
		country:    country,
		adapter:    adapter,
		matcher:    matcher,
		recodeSize: size,
	}

	return geoSite, nil
}

var _ C.Rule = (*GEOSITE)(nil)
