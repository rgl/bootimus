//go:build !linux

package netbind

import (
	"fmt"
	"syscall"
)

func bindToDevice(device string) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		return fmt.Errorf("binding to interface %s is only supported on Linux", device)
	}
}
