//
// Copyright 2014 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

import "io"

// SerialPort object
type SerialPort interface {
	// Read(p []byte) (n int, err error)
	// Write(p []byte) (n int, err error)
	// Close() error
	io.ReadWriteCloser

	// Set port speed
	SetSpeed(baudrate int) error

	// Set port parity
	SetParity(parity Parity) error
}

type Parity int

const (
	PARITY_NONE Parity = iota
	PARITY_ODD
	PARITY_EVEN
	PARITY_MARK
	PARITY_SPACE
)

// Platform independent error type for serial ports
type SerialPortError struct {
	err  string
	code int
}

const (
	ERROR_PORT_BUSY           = 1
	ERROR_PORT_NOT_FOUND      = 2
	ERROR_INVALID_SERIAL_PORT = 3
	ERROR_PERMISSION_DENIED   = 4
	ERROR_INVALID_PORT_SPEED  = 5
	ERROR_ENUMERATING_PORTS   = 6
	ERROR_OTHER               = 99
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
	case ERROR_ENUMERATING_PORTS:
		return "Could not enumerate serial ports"
	}
	return e.err
}

func (e SerialPortError) Code() int {
	return e.code
}

// vi:ts=2
