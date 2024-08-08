//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

import (
	"golang.org/x/sys/unix"
	"math"
	"regexp"
)

const devFolder = "/dev"

// see tty(4), ucom(4), zstty(4), ...
var osPortFilter = regexp.MustCompile("^([dt]ty[a-d]|[dt]ty[0-9]+|[dt]ty[CBZ][0-1]|[dt]tyU[0-9]+)$")

// termios manipulation functions

var databitsMap = map[int]uint32{
	0: unix.CS8, // Default to 8 bits
	5: unix.CS5,
	6: unix.CS6,
	7: unix.CS7,
	8: unix.CS8,
}

const tcCMSPAR uint32 = 0 // not supported
const tcIUCLC uint32 = 0  // not supported

const tcCRTSCTS uint32 = unix.CRTSCTS

const ioctlTcgetattr = unix.TIOCGETA
const ioctlTcsetattr = unix.TIOCSETA
const ioctlTcflsh = unix.TIOCFLUSH
const ioctlTioccbrk = unix.TIOCCBRK
const ioctlTiocsbrk = unix.TIOCSBRK

func setTermSettingsBaudrate(speed int, settings *unix.Termios) (error, bool) {
	// see https://nxr.netbsd.org/xref/src/lib/libc/termios/cfsetspeed.c
	if speed < 50 || speed > math.MaxInt32 {
		return &PortError{code: InvalidSpeed}, true
	}
	settings.Ispeed = int32(speed)
	settings.Ospeed = int32(speed)
	return nil, false
}

func (port *unixPort) setSpecialBaudrate(speed uint32) error {
	if speed < 50 || speed > math.MaxInt32 {
		return &PortError{code: InvalidSpeed}
	}
	// see https://nxr.netbsd.org/xref/src/lib/libc/termios/tcgetattr.c
	settings, err := unix.IoctlGetTermios(port.handle, ioctlTcgetattr)
	if err != nil {
		return err
	}
	if err, ok := setTermSettingsBaudrate(int(speed), settings); !ok {
		return err
	}
	return unix.IoctlSetTermios(port.handle, ioctlTcsetattr, settings)
}
