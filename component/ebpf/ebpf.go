package ebpf

import (
	"net/netip"

	C "github.com/c83a/Clash.Meta/constant"
	"github.com/c83a/Clash.Meta/transport/socks5"
)

type TcEBpfProgram struct {
	pros    []C.EBpf
	rawNICs []string
}

func (t *TcEBpfProgram) RawNICs() []string {
	return t.rawNICs
}

func (t *TcEBpfProgram) Close() {
	for _, p := range t.pros {
		p.Close()
	}
}

func (t *TcEBpfProgram) Lookup(srcAddrPort netip.AddrPort) (addr socks5.Addr, err error) {
	for _, p := range t.pros {
		addr, err = p.Lookup(srcAddrPort)
		if err == nil {
			return
		}
	}
	return
}
