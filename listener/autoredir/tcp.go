package autoredir

import (
	"net"
	"net/netip"

	"github.com/c83a/Clash.Meta/adapter/inbound"
	N "github.com/c83a/Clash.Meta/common/net"
	C "github.com/c83a/Clash.Meta/constant"
	"github.com/c83a/Clash.Meta/log"
	"github.com/c83a/Clash.Meta/transport/socks5"
)

type Listener struct {
	listener   net.Listener
	addr       string
	closed     bool
	additions  []inbound.Addition
	lookupFunc func(netip.AddrPort) (socks5.Addr, error)
}

// RawAddress implements C.Listener
func (l *Listener) RawAddress() string {
	return l.addr
}

// Address implements C.Listener
func (l *Listener) Address() string {
	return l.listener.Addr().String()
}

// Close implements C.Listener
func (l *Listener) Close() error {
	l.closed = true
	return l.listener.Close()
}

func (l *Listener) TCPAddr() netip.AddrPort {
	return l.listener.Addr().(*net.TCPAddr).AddrPort()
}

func (l *Listener) SetLookupFunc(lookupFunc func(netip.AddrPort) (socks5.Addr, error)) {
	l.lookupFunc = lookupFunc
}

func (l *Listener) handleRedir(conn net.Conn, tunnel C.Tunnel) {
	if l.lookupFunc == nil {
		log.Errorln("[Auto Redirect] lookup function is nil")
		return
	}

	target, err := l.lookupFunc(conn.RemoteAddr().(*net.TCPAddr).AddrPort())
	if err != nil {
		log.Warnln("[Auto Redirect] %v", err)
		_ = conn.Close()
		return
	}

	N.TCPKeepAlive(conn)

	tunnel.HandleTCPConn(inbound.NewSocket(target, conn, C.REDIR, l.additions...))
}

func New(addr string, tunnel C.Tunnel, additions ...inbound.Addition) (*Listener, error) {
	if len(additions) == 0 {
		additions = []inbound.Addition{
			inbound.WithInName("DEFAULT-REDIR"),
			inbound.WithSpecialRules(""),
		}
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	rl := &Listener{
		listener:  l,
		addr:      addr,
		additions: additions,
	}

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				if rl.closed {
					break
				}
				continue
			}
			go rl.handleRedir(c, tunnel)
		}
	}()

	return rl, nil
}
