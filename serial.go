//
// Copyright 2014-2016 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial // import "go.bug.st/serial"

// This structure describes a serial port configuration.
type Mode struct {
	BaudRate int      // The serial port bitrate (aka Baudrate)
	DataBits int      // Size of the character (must be 5, 6, 7 or 8)
	Parity   Parity   // Parity (see Parity type for more info)
	StopBits StopBits // Stop bits (see StopBits type for more info)
}

type Parity int

const (
	PARITY_NONE  Parity = iota // No parity (default)
	PARITY_ODD                 // Odd parity
	PARITY_EVEN                // Even parity
	PARITY_MARK                // Mark parity (always 1)
	PARITY_SPACE               // Space parity (always 0)
)

type StopBits int

const (
	STOPBITS_ONE          StopBits = iota // 1 Stop bit
	STOPBITS_ONEPOINTFIVE                 // 1.5 Stop bits
	STOPBITS_TWO                          // 2 Stop bits
)

// Platform independent error type for serial ports
type SerialPortError struct {
	err  string
	code int
}

const (
	ERROR_PORT_BUSY = iota
	ERROR_PORT_NOT_FOUND
	ERROR_INVALID_SERIAL_PORT
	ERROR_PERMISSION_DENIED
	ERROR_INVALID_PORT_SPEED
	ERROR_INVALID_PORT_DATA_BITS
	ERROR_ENUMERATING_PORTS
	ERROR_OTHER
)

func (e SerialPortError) Error() string {
	switch e.code {
	case ERROR_PORT_BUSY:
		return "Serial port busy"
	case ERROR_PORT_NOT_FOUND:
		return "Serial port not found"
	case ERROR_INVALID_SERIAL_PORT:
		return "Invalid serial port"
	case ERROR_PERMISSION_DENIED:
		return "Permission denied"
	case ERROR_INVALID_PORT_SPEED:
		return "Invalid port speed"
	case ERROR_INVALID_PORT_DATA_BITS:
		return "Invalid port data bits"
	case ERROR_ENUMERATING_PORTS:
		return "Could not enumerate serial ports"
	}
	return e.err
}

func (e SerialPortError) Code() int {
	return e.code
}

