package constant

import (
	"github.com/c83a/Clash.Meta/component/geodata/router"
)

type RuleGeoSite interface {
	GetDomainMatcher() router.DomainMatcher
}

type RuleGeoIP interface {
	GetIPMatcher() *router.GeoIPMatcher
}

type RuleGroup interface {
	GetRecodeSize() int
}
