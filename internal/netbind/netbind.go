package netbind

import (
	"context"
	"fmt"
	"net"
)

func ListenConfig(device string) net.ListenConfig {
	if device == "" {
		return net.ListenConfig{}
	}
	return net.ListenConfig{Control: bindToDevice(device)}
}

func Listen(device, network, addr string) (net.Listener, error) {
	lc := ListenConfig(device)
	return lc.Listen(context.Background(), network, addr)
}

func ListenUDP(device, network, addr string) (*net.UDPConn, error) {
	lc := ListenConfig(device)
	pc, err := lc.ListenPacket(context.Background(), network, addr)
	if err != nil {
		return nil, err
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		pc.Close()
		return nil, fmt.Errorf("expected UDP connection, got %T", pc)
	}
	return conn, nil
}

func InterfaceIPv4(device string) (net.IP, error) {
	iface, err := net.InterfaceByName(device)
	if err != nil {
		return nil, fmt.Errorf("interface %s: %w", device, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("interface %s addresses: %w", device, err)
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			return ip4, nil
		}
	}
	return nil, fmt.Errorf("interface %s has no IPv4 address", device)
}
