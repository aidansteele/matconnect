package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"github.com/lucas-clemente/quic-go"
	"net"
	"net/netip"
)

type matdialer struct {
	ip netip.Addr
}

func (sd *matdialer) Dial(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
	remoteAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolving udp addr: %w", err)
	}

	pconn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, fmt.Errorf("net.DialUDP: %w", err)
	}

	localAddr := pconn.LocalAddr().(*net.UDPAddr)

	buf := &bytes.Buffer{}
	buf.Write(sd.ip.AsSlice())
	binary.Write(buf, binary.BigEndian, uint16(localAddr.Port))
	_, err = pconn.WriteToUDP(buf.Bytes(), remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("writeToUdp: %w", err)
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("splitting host and port (%s): %w", addr, err)
	}

	return quic.DialEarlyContext(ctx, pconn, remoteAddr, host, tlsCfg, cfg)
}

type matconn struct {
	net.PacketConn
	addrMap map[string]net.Addr
}

func newMatconn(packetConn net.PacketConn) *matconn {
	return &matconn{
		PacketConn: packetConn,
		addrMap:    map[string]net.Addr{},
	}
}

func (s *matconn) remap(buf []byte, oldAddr net.Addr) error {
	if len(buf) != 6 {
		return fmt.Errorf("short read: %d bytes read", len(buf))
	}

	destIp := net.IPv4(buf[0], buf[1], buf[2], buf[3])
	destPort := binary.BigEndian.Uint16(buf[4:])
	newAddr := &net.UDPAddr{IP: destIp, Port: int(destPort)}

	s.addrMap[oldAddr.String()] = newAddr
	fmt.Printf("mapped %s to %s\n", oldAddr, newAddr)
	return nil
}

func (s *matconn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = s.PacketConn.ReadFrom(p)

	if n == 6 {
		err = s.remap(p[:6], addr)
		if err != nil {
			return -1, addr, fmt.Errorf("reading init packet: %w", err)
		}

		n, addr, err = s.PacketConn.ReadFrom(p)
	}

	return n, addr, err
}

func (s *matconn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	newAddr, ok := s.addrMap[addr.String()]
	if !ok {
		return 0, fmt.Errorf("couldn't find remapped addr to send to for %s", addr)
	}

	n, err = s.PacketConn.WriteTo(p, newAddr)
	return n, err
}
