//
// Copyright 2014-2017 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator // import "go.bug.st/serial.v1/enumerator"

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func parseAndReturnDeviceID(deviceID string) *PortDetails {
	res := &PortDetails{}
	parseDeviceID(deviceID, res)
	return res
}

func TestParseDeviceID(t *testing.T) {
	r := require.New(t)
	test := func(deviceId, vid, pid, serialNo string) {
		res := parseAndReturnDeviceID(deviceId)
		r.True(res.IsUSB)
		r.Equal(vid, res.VID)
		r.Equal(pid, res.PID)
		r.Equal(serialNo, res.SerialNumber)
	}

	test("FTDIBUS\\VID_0403+PID_6001+A6004CCFA\\0000", "0403", "6001", "A6004CCFA")
	test("USB\\VID_16C0&PID_0483\\12345", "16C0", "0483", "12345")
	test("USB\\VID_2341&PID_0000\\64936333936351400000", "2341", "0000", "64936333936351400000")
	test("USB\\VID_2341&PID_0000\\6493234373835191F1F1", "2341", "0000", "6493234373835191F1F1")
	test("USB\\VID_2341&PID_804E&MI_00\\6&279A3900&0&0000", "2341", "804E", "")
	test("USB\\VID_2341&PID_004E\\5&C3DC240&0&1", "2341", "004E", "")
	test("USB\\VID_03EB&PID_2111&MI_01\\6&21F3553F&0&0001", "03EB", "2111", "") // Atmel EDBG
	test("USB\\VID_2341&PID_804D&MI_00\\6&1026E213&0&0000", "2341", "804D", "")
	test("USB\\VID_2341&PID_004D\\5&C3DC240&0&1", "2341", "004D", "")
	test("USB\\VID_067B&PID_2303\\6&2C4CB384&0&3", "067B", "2303", "") // PL2303
}

func TestParseDeviceIDWithInvalidStrings(t *testing.T) {
	r := require.New(t)
	res := parseAndReturnDeviceID("ABC")
	r.False(res.IsUSB)
	res2 := parseAndReturnDeviceID("USB")
	r.False(res2.IsUSB)
}
