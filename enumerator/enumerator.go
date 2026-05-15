//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output syscall_windows.go usb_windows.go

// PortDetails contains detailed information about USB serial port.
// Use GetDetailedPortsList function to retrieve it.
type PortDetails struct {
	// Name is the port address, like COM1 on Windows or /dev/ttyUSB0 on Linux.
	Name string
	// IsUSB is true if the port is a USB serial port, false otherwise.
	IsUSB bool
	// VID is the USB Vendor ID, when available.
	VID string
	// PID is the USB Product ID, when available.
	PID string
	// SerialNumber is the USB serial number, when available.
	SerialNumber string
	// Configuration is the USB configuration string, when available.
	Configuration string
	// Manufacturer is the USB iManufacturer string, when available.
	Manufacturer string
	// Product is the USB iProduct string, when available.
	Product string
}

// GetDetailedPortsList retrieve ports details like USB VID/PID.
// Please note that this function may not be available on all OS:
// in that case a FunctionNotImplemented error is returned.
func GetDetailedPortsList() ([]*PortDetails, error) {
	return nativeGetDetailedPortsList()
}

// PortEnumerationError is the error type for serial ports enumeration
type PortEnumerationError struct {
	causedBy error
}

// Error returns the complete error code with details on the cause of the error
func (e PortEnumerationError) Error() string {
	reason := "Error while enumerating serial ports"
	if e.causedBy != nil {
		reason += ": " + e.causedBy.Error()
	}
	return reason
}
