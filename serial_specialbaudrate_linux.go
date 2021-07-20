// +build linux
// +build !ppc64le

//
// Copyright 2014-2021 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

import "golang.org/x/sys/unix"

func (port *unixPort) setSpecialBaudrate(speed uint32) error {
	settings, err := unix.IoctlGetTermios(port.handle, unix.TCGETS2)
	if err != nil {
		return err
	}
	settings.Cflag &^= unix.CBAUD
	settings.Cflag |= unix.BOTHER
	settings.Ispeed = speed
	settings.Ospeed = speed
	return unix.IoctlSetTermios(port.handle, unix.TCSETS2, settings)
}
