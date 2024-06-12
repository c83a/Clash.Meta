package inbound

import (
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"github.com/metacubex/mihomo/common/nnip"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
)
var mPool sync.Pool
func init(){
	mPool = sync.Pool{New :func()any{
		return &C.Metadata{}
		},
	}
}
func Getm() *C.Metadata{
	m:=mPool.Get().(*C.Metadata)
	if m.DstPort > 0{
	m.DstGeoIP = nil
	m.SrcPort = 0
	m.DstPort = 0
	m.InPort = 0
	m.DSCP = 0
	m.Uid = 0
		s:= ""
	m.DstIPASN = s
	m.InName = s
	m.InUser = s
	m.Host = s
	m.Process = s
	m.ProcessPath = s
	m.SpecialProxy = s
	m.SpecialRules = s
	m.RemoteDst = s
	m.SniffHost = s
	/*
	SrcPort      uint16     `json:"sourcePort,string"`      // `,string` is used to compatible with old version json output
	DstPort      uint16     `json:"destinationPort,string"` // `,string` is used to compatible with old version json output
	InPort       uint16     `json:"inboundPort,string"` // `,string` is used to compatible with old version json output
	DSCP         uint8      `json:"dscp"`
	Uid          uint32     `json:"uid"`
	DstIPASN     string     `json:"destinationIPASN"`
	InName       string     `json:"inboundName"`
	InUser       string     `json:"inboundUser"`
	Host         string     `json:"host"`
	Process      string     `json:"process"`
	ProcessPath  string     `json:"processPath"`
	SpecialProxy string     `json:"specialProxy"`
	SpecialRules string     `json:"specialRules"`
	RemoteDst    string     `json:"remoteDestination"`
	// Only domain rule
	SniffHost string `json:"sniffHost"`
	// clean
	*/
	}
	return m
}

func Putm(m *C.Metadata){
	mPool.Put(m)
}
func parseSocksAddr(target socks5.Addr) *C.Metadata {
	metadata := Getm()
	switch target[0] {
	case socks5.AtypDomainName:
		// trim for FQDN
		metadata.Host = strings.TrimRight(string(target[2:2+target[1]]), ".")
		metadata.DstPort = uint16((int(target[2+target[1]]) << 8) | int(target[2+target[1]+1]))
	case socks5.AtypIPv4:
		metadata.DstIP = nnip.IpToAddr(net.IP(target[1 : 1+net.IPv4len]))
		metadata.DstPort = uint16((int(target[1+net.IPv4len]) << 8) | int(target[1+net.IPv4len+1]))
	case socks5.AtypIPv6:
		ip6, _ := netip.AddrFromSlice(target[1 : 1+net.IPv6len])
		metadata.DstIP = ip6.Unmap()
		metadata.DstPort = uint16((int(target[1+net.IPv6len]) << 8) | int(target[1+net.IPv6len+1]))
	}

	return metadata
}

func parseHTTPAddr(request *http.Request) *C.Metadata {
	host := request.URL.Hostname()
	port := request.URL.Port()
	if port == "" {
		port = "80"
	}

	// trim FQDN (#737)
	host = strings.TrimRight(host, ".")

	var uint16Port uint16
	if port, err := strconv.ParseUint(port, 10, 16); err == nil {
		uint16Port = uint16(port)
	}

	m := Getm()
	m.NetWork=C.TCP
	m.Host=host
	m.DstPort= uint16Port
/*	metadata := &C.Metadata{
		NetWork: C.TCP,
		Host:    host,
		DstIP:   netip.Addr{},
		DstPort: uint16Port,
	}
*/
	ip, err := netip.ParseAddr(host)
	if err == nil {
		m.DstIP = ip
	}else{
		m.DstIP = netip.Addr{}
	}

	return m
}
