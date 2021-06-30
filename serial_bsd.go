// +build darwin dragonfly freebsd netbsd openbsd

package serial

import "golang.org/x/sys/unix"

func (port *unixPort) Drain() error {
	return ioctl(port.handle, unix.TIOCDRAIN, 0)
}
