//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

import (
	"testing"
)

func parseAndReturnDeviceID(deviceID string) *PortDetails {
	res := &PortDetails{}
	parseDeviceID(deviceID, res)
	return res
}

func TestParseDeviceID(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		vid      string
		pid      string
		serialNo string
	}{
		{name: "FTDI FT232", deviceID: "FTDIBUS\\VID_0403+PID_6001+A6004CCFA\\0000", vid: "0403", pid: "6001", serialNo: "A6004CCFA"},
		{name: "Teensy USB serial", deviceID: "USB\\VID_16C0&PID_0483\\12345", vid: "16C0", pid: "0483", serialNo: "12345"},
		{name: "Arduino with serial number", deviceID: "USB\\VID_2341&PID_0000\\64936333936351400000", vid: "2341", pid: "0000", serialNo: "64936333936351400000"},
		{name: "Arduino with different serial number", deviceID: "USB\\VID_2341&PID_0000\\6493234373835191F1F1", vid: "2341", pid: "0000", serialNo: "6493234373835191F1F1"},
		{name: "Arduino MKR composite", deviceID: "USB\\VID_2341&PID_804E&MI_00\\6&279A3900&0&0000", vid: "2341", pid: "804E", serialNo: ""},
		{name: "Arduino MKR1000 bootloader", deviceID: "USB\\VID_2341&PID_004E\\5&C3DC240&0&1", vid: "2341", pid: "004E", serialNo: ""},
		{name: "Atmel EDBG debugger", deviceID: "USB\\VID_03EB&PID_2111&MI_01\\6&21F3553F&0&0001", vid: "03EB", pid: "2111", serialNo: ""},
		{name: "Arduino Zero composite", deviceID: "USB\\VID_2341&PID_804D&MI_00\\6&1026E213&0&0000", vid: "2341", pid: "804D", serialNo: ""},
		{name: "Arduino Zero bootloader", deviceID: "USB\\VID_2341&PID_004D\\5&C3DC240&0&1", vid: "2341", pid: "004D", serialNo: ""},
		{name: "Prolific PL2303", deviceID: "USB\\VID_067B&PID_2303\\6&2C4CB384&0&3", vid: "067B", pid: "2303", serialNo: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := parseAndReturnDeviceID(tt.deviceID)
			if !res.IsUSB {
				t.Fatal("expected IsUSB to be true")
			}
			if res.VID != tt.vid {
				t.Errorf("VID: got %q, expected %q", res.VID, tt.vid)
			}
			if res.PID != tt.pid {
				t.Errorf("PID: got %q, expected %q", res.PID, tt.pid)
			}
			if res.SerialNumber != tt.serialNo {
				t.Errorf("SerialNumber: got %q, expected %q", res.SerialNumber, tt.serialNo)
			}
		})
	}
}

func TestParseDeviceIDWithInvalidStrings(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
	}{
		{name: "unrecognized prefix", deviceID: "ABC"},
		{name: "USB prefix without fields", deviceID: "USB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := parseAndReturnDeviceID(tt.deviceID)
			if res.IsUSB {
				t.Fatal("expected IsUSB to be false")
			}
		})
	}
}
