//go:build linux

package main

import "syscall"

// makeDialControl — set SO_RCVBUF/SO_SNDBUF + TCP keepalive di raw socket.
func makeDialControl(rcvbuf int) func(network, address string, c syscall.RawConn) error {
	if rcvbuf <= 0 {
		return nil
	}
	return func(network, address string, c syscall.RawConn) error {
		var sockErr error
		c.Control(func(fd uintptr) {
			if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, rcvbuf); err != nil && sockErr == nil {
				sockErr = err
			}
			if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, rcvbuf); err != nil && sockErr == nil {
				sockErr = err
			}
			_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)
			_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, 60)
			_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, 15)
			_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, 4)
			_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)
		})
		return sockErr
	}
}
