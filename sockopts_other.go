//go:build !linux

package main

import "syscall"

// Non-Linux fallback: hanya enable SO_KEEPALIVE (opsi TCP_KEEP* tidak portabel).
func makeDialControl(rcvbuf int) func(network, address string, c syscall.RawConn) error {
	if rcvbuf <= 0 {
		return nil
	}
	return func(network, address string, c syscall.RawConn) error {
		c.Control(func(fd uintptr) {
			_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)
		})
		return nil
	}
}
