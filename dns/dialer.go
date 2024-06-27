package dns

// export functions from tunnel module

import "github.com/c83a/Clash.Meta/tunnel"

const RespectRules = tunnel.DnsRespectRules

type dnsDialer = tunnel.DNSDialer

var newDNSDialer = tunnel.NewDNSDialer
