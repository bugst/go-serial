//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

func nativeGetDetailedPortsList() ([]*PortDetails, error) {
	/*
	 * TODO:
	 * It should open dev/usbN and issue the DRVLISTDEV ioctl.
	 * see https://nxr.netbsd.org/xref/src/usr.sbin/usbdevs/usbdevs.c
	 */
	return nil, &PortEnumerationError{}
}
