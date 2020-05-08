//
// Copyright 2014-2020 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

import "time"

//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsyscall_windows.go syscall_windows.go

// Port is the interface for a serial Port
type Port interface {
	// SetMode sets all parameters of the serial port
	SetMode(mode *Mode) error

	// Stores data received from the serial port into the provided byte array
	// buffer. The function returns the number of bytes read.
	//
	// The Read function blocks until (at least) one byte is received from
	// the serial port or an error occurs.
	Read(p []byte) (n int, err error)

	// Send the content of the data byte array to the serial port.
	// Returns the number of bytes written.
	Write(p []byte) (n int, err error)

	// ResetInputBuffer Purges port read buffer
	ResetInputBuffer() error

	// ResetOutputBuffer Purges port write buffer
	ResetOutputBuffer() error

	// SetDTR sets the modem status bit DataTerminalReady
	SetDTR(dtr bool) error

	// SetRTS sets the modem status bit RequestToSend
	SetRTS(rts bool) error

	// GetModemStatusBits returns a ModemStatusBits structure containing the
	// modem status bits for the serial port (CTS, DSR, etc...)
	GetModemStatusBits() (*ModemStatusBits, error)

	// Close the serial port
	Close() error
}

// ModemStatusBits contains all the modem status bits for a serial port (CTS, DSR, etc...).
// It can be retrieved with the Port.GetModemStatusBits() method.
type ModemStatusBits struct {
	CTS bool // ClearToSend status
	DSR bool // DataSetReady status
	RI  bool // RingIndicator status
	DCD bool // DataCarrierDetect status
}

// Open opens the serial port using the specified modes
func Open(portName string, mode *Mode) (Port, error) {
	return nativeOpen(portName, mode)
}

// GetPortsList retrieve the list of available serial ports
func GetPortsList() ([]string, error) {
	return nativeGetPortsList()
}

// Mode describes a serial port configuration.
type Mode struct {
	BaudRate int      // The serial port bitrate (aka Baudrate)
	DataBits int      // Size of the character (must be 5, 6, 7 or 8)
	Parity   Parity   // Parity (see Parity type for more info)
	StopBits StopBits // Stop bits (see StopBits type for more info)

	RS485 RS485Config // RS485 configuration
}

// Parity describes a serial port parity setting
type Parity int

const (
	// NoParity disable parity control (default)
	NoParity Parity = iota
	// OddParity enable odd-parity check
	OddParity
	// EvenParity enable even-parity check
	EvenParity
	// MarkParity enable mark-parity (always 1) check
	MarkParity
	// SpaceParity enable space-parity (always 0) check
	SpaceParity
)

// StopBits describe a serial port stop bits setting
type StopBits int

const (
	// OneStopBit sets 1 stop bit (default)
	OneStopBit StopBits = iota
	// OnePointFiveStopBits sets 1.5 stop bits
	OnePointFiveStopBits
	// TwoStopBits sets 2 stop bits
	TwoStopBits
)

// RS485Config -- platform independent RS485 config. Thie structure is ignored unless Enable is true.
type RS485Config struct {
	// Enable RS485 support
	Enabled bool
	// Delay RTS prior to send
	DelayRtsBeforeSend time.Duration
	// Delay RTS after send
	DelayRtsAfterSend time.Duration
	// Set RTS high during send
	RtsHighDuringSend bool
	// Set RTS high after send
	RtsHighAfterSend bool
	// Rx during Tx
	RxDuringTx bool
}

// PortError is a platform independent error type for serial ports
type PortError struct {
	code     PortErrorCode
	causedBy error
}

// PortErrorCode is a code to easily identify the type of error
type PortErrorCode int

const (
	// PortBusy the serial port is already in used by another process
	PortBusy PortErrorCode = iota
	// PortNotFound the requested port doesn't exist
	PortNotFound
	// InvalidSerialPort the requested port is not a serial port
	InvalidSerialPort
	// PermissionDenied the user doesn't have enough priviledges
	PermissionDenied
	// InvalidSpeed the requested speed is not valid or not supported
	InvalidSpeed
	// InvalidDataBits the number of data bits is not valid or not supported
	InvalidDataBits
	// InvalidParity the selected parity is not valid or not supported
	InvalidParity
	// InvalidStopBits the selected number of stop bits is not valid or not supported
	InvalidStopBits
	// ErrorEnumeratingPorts an error occurred while listing serial port
	ErrorEnumeratingPorts
	// PortClosed the port has been closed while the operation is in progress
	PortClosed
	// FunctionNotImplemented the requested function is not implemented
	FunctionNotImplemented
	// ReadFailed indicates the read failed
	ReadFailed
	// ConfigureRS485Error indicates an error configuring RS485 on the platform
	ConfigureRS485Error
	// NoPlatformSupportForRS485 indicates no platform support for RS485
	NoPlatformSupportForRS485
)

// EncodedErrorString returns a string explaining the error code
func (e PortError) EncodedErrorString() string {
	switch e.code {
	case PortBusy:
		return "Serial port busy"
	case PortNotFound:
		return "Serial port not found"
	case InvalidSerialPort:
		return "Invalid serial port"
	case PermissionDenied:
		return "Permission denied"
	case InvalidSpeed:
		return "Port speed invalid or not supported"
	case InvalidDataBits:
		return "Port data bits invalid or not supported"
	case InvalidParity:
		return "Port parity invalid or not supported"
	case InvalidStopBits:
		return "Port stop bits invalid or not supported"
	case ErrorEnumeratingPorts:
		return "Could not enumerate serial ports"
	case PortClosed:
		return "Port has been closed"
	case FunctionNotImplemented:
		return "Function not implemented"
	case ReadFailed:
		return "Read failed"
	case ConfigureRS485Error:
		return "Error configuring RS485 on the platform"
	case NoPlatformSupportForRS485:
		return "Platform does not support RS485"
	default:
		return "Other error"
	}
}

// Error returns the complete error code with details on the cause of the error
func (e PortError) Error() string {
	if e.causedBy != nil {
		return e.EncodedErrorString() + ": " + e.causedBy.Error()
	}
	return e.EncodedErrorString()
}

// Code returns an identifier for the kind of error occurred
func (e PortError) Code() PortErrorCode {
	return e.code
}
