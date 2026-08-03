//go:build linux

package netbind

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func bindToDevice(device string) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		var sockErr error
		err := c.Control(func(fd uintptr) {
			sockErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, device)
		})
		if err != nil {
			return err
		}
		return sockErr
	}
}
