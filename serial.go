//
// Copyright 2014-2017 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial // import "go.bug.st/serial.v1"

//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsyscall_windows.go syscall_windows.go

// Port is the interface for a serial Port
type Port interface {
	// SetMode sets all parameters of the serial port
	SetMode(mode *Mode) error

	// SetReadTimeout sets the whole packet read timeout.
	// Values:
	//   t < 0: Blocking mode
	// 			`Read` function wait until requested number of bytes are received (possible forever).
	//   t = 0: Non-blocking mode
	// 			`Read` function returns immediately in any case, returning up to the requested number of bytes.
	//   t > 0: set timeout to `t` milliseconds.
	// 			`Read` function returns immediately when the requested number of bytes are available,
	//          otherwise wait until the timeout expires and return all bytes that were received until them.
	SetReadTimeout(t int) error

	// SetReadTimeoutEx — Sets whole package read timeout similar to general purpose function SetReadTimeout(),
	// and also sets interbyte timeout.
	//
	// Generally interbyte timeout is not needed, but in some special cases this function cat help you.
	SetReadTimeoutEx(t, i uint32) error

	// SetLegacyReadTimeout — Very special function.
	//
	// Based on https://msdn.microsoft.com/ru-ru/library/windows/desktop/aa363190(v=vs.85).aspx:
	// If there are any bytes in the input buffer, ReadFile returns immediately with the bytes in the buffer.
	// If there are no bytes in the input buffer, ReadFile waits until a byte arrives and then returns immediately.
	// If no bytes arrive within the time specified by ReadTotalTimeoutConstant, ReadFile times out.
	//
	// Use it to configure read timeout in legacy manner. (Legacy for this library).
	SetFirstByteReadTimeout(t uint32) error

	// SetWriteTimeout set whole packet write timeout
	// Values:
	// Values:
	//   t < 0: Blocking mode
	// 			`Write` function will block until complete or error.
	// 			Depending of OS layer it can call multiple subsequent os-level write calls until done.
	//   t = 0: Non-blocking mode
	// 			`Write` function will write some data and returns even not all data has been written.
	//          Depending of OS layer it makes only signle subsequent os-level write call.
	//   t > 0: set timeout to `t` milliseconds.
	// 			`Write` function will write untile complete, error or timeout.
	// 			Depending of OS layer it can call multiple subsequent os-levek write calls until done.
	SetWriteTimeout(t int) error

	// Stores data received from the serial port into the provided byte array
	// buffer. The function returns the number of bytes read.
	//
	// The Read function blocks until (at least) one byte is received from
	// the serial port or a timeout reached or an error occurs.
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
	// Invalid timeout value passed
	InvalidTimeoutValue
	// ErrorEnumeratingPorts an error occurred while listing serial port
	ErrorEnumeratingPorts
	// PortClosed the port has been closed while the operation is in progress
	PortClosed
	// FunctionNotImplemented the requested function is not implemented
	FunctionNotImplemented
	// Operating system function error
	OsError
	// Port write failed
	WriteFailed
	// Port read failed
	ReadFailed
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
	case InvalidTimeoutValue:
		return "Timeout value invalid or not supported"
	case ErrorEnumeratingPorts:
		return "Could not enumerate serial ports"
	case PortClosed:
		return "Port has been closed"
	case FunctionNotImplemented:
		return "Function not implemented"
	case OsError:
		return "Operating system error"
	case WriteFailed:
		return "Write failed"
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
