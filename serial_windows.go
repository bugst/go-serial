//
// Copyright 2014-2017 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial // import "go.bug.st/serial.v1"

/*

// MSDN article on Serial Communications:
// http://msdn.microsoft.com/en-us/library/ff802693.aspx
// (alternative link) https://msdn.microsoft.com/en-us/library/ms810467.aspx

// PySerial source code and docs:
// https://github.com/pyserial/pyserial
// https://pythonhosted.org/pyserial/

// Arduino Playground article on serial communication with Windows API:
// http://playground.arduino.cc/Interfacing/CPPWindows

*/

import "syscall"

type windowsPort struct {
	handle   syscall.Handle
	mode     *Mode
	timeouts *commTimeouts
}

func nativeGetPortsList() ([]string, error) {
	subKey, err := syscall.UTF16PtrFromString("HARDWARE\\DEVICEMAP\\SERIALCOMM\\")
	if err != nil {
		return nil, &PortError{code: ErrorEnumeratingPorts}
	}

	var h syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, subKey, 0, syscall.KEY_READ, &h) != nil {
		return nil, &PortError{code: ErrorEnumeratingPorts}
	}
	defer syscall.RegCloseKey(h)

	var valuesCount uint32
	if syscall.RegQueryInfoKey(h, nil, nil, nil, nil, nil, nil, &valuesCount, nil, nil, nil, nil) != nil {
		return nil, &PortError{code: ErrorEnumeratingPorts}
	}

	list := make([]string, valuesCount)
	for i := range list {
		var data [1024]uint16
		dataSize := uint32(len(data))
		var name [1024]uint16
		nameSize := uint32(len(name))
		if regEnumValue(h, uint32(i), &name[0], &nameSize, nil, nil, &data[0], &dataSize) != nil {
			return nil, &PortError{code: ErrorEnumeratingPorts}
		}
		list[i] = syscall.UTF16ToString(data[:])
	}
	return list, nil
}

func (port *windowsPort) Close() error {
	h := port.handle
	if h == syscall.InvalidHandle {
		return nil
	}
	port.handle = syscall.InvalidHandle
	return syscall.CloseHandle(h)
}

func (port *windowsPort) Read(p []byte) (int, error) {
	if port.handle == syscall.InvalidHandle {
		return 0, &PortError{code: PortClosed, causedBy: nil}
	}
	handle := port.handle

	errs := new(uint32)
	stat := new(comstat)
	if err := clearCommError(handle, errs, stat); err != nil {
		return 0, &PortError{code: InvalidSerialPort, causedBy: err}
	}

	size := uint32(len(p))
	var readSize uint32
	if port.timeouts.ReadTotalTimeoutConstant == 0 && port.timeouts.ReadTotalTimeoutMultiplier == 0 {
		if stat.inque < size {
			readSize = stat.inque
		} else {
			readSize = size
		}
	} else {
		readSize = size
	}

	if readSize > 0 {
		var read uint32
		overlapped, err := createOverlappedStruct()
		if err != nil {
			return 0, &PortError{code: OsError, causedBy: err}
		}
		defer syscall.CloseHandle(overlapped.HEvent)
		err = syscall.ReadFile(handle, p[:readSize], &read, overlapped)
		if err != nil && err != syscall.ERROR_IO_PENDING {
			return 0, &PortError{code: OsError, causedBy: err}
		}
		err = getOverlappedResult(handle, overlapped, &read, true)
		if err != nil && err != syscall.ERROR_OPERATION_ABORTED {
			return 0, &PortError{code: OsError, causedBy: err}
		}
		return int(read), nil
	} else {
		return 0, nil
	}
}

func (port *windowsPort) Write(p []byte) (int, error) {
	if port.handle == syscall.InvalidHandle {
		return 0, &PortError{code: PortClosed, causedBy: nil}
	}
	handle := port.handle

	errs := new(uint32)
	stat := new(comstat)
	if err := clearCommError(handle, errs, stat); err != nil {
		return 0, &PortError{code: InvalidSerialPort, causedBy: err}
	}

	overlapped, err := createOverlappedStruct()
	if err != nil {
		return 0, err
	}
	defer syscall.CloseHandle(overlapped.HEvent)
	var written uint32
	err = syscall.WriteFile(handle, p, &written, overlapped)
	if err == nil || err == syscall.ERROR_IO_PENDING || err == syscall.ERROR_OPERATION_ABORTED {
		err = getOverlappedResult(handle, overlapped, &written, true)
		if err == nil || err == syscall.ERROR_OPERATION_ABORTED {
			return int(written), nil
		}
	}
	return int(written), err
}

func (port *windowsPort) ResetInputBuffer() error {
	return purgeComm(port.handle, purgeRxClear|purgeRxAbort)
}

func (port *windowsPort) ResetOutputBuffer() error {
	return purgeComm(port.handle, purgeTxClear|purgeTxAbort)
}

func (port *windowsPort) SetMode(mode *Mode) error {
	port.mode = mode
	return port.reconfigurePort()
}

func (port *windowsPort) SetDTR(dtr bool) error {
	// Like for RTS there are problems with the escapeCommFunction
	// observed behaviour was that DTR is set from false -> true
	// when setting RTS from true -> false
	// 1) Connect 		-> RTS = true 	(low) 	DTR = true 	(low) 	OKAY
	// 2) SetDTR(false) -> RTS = true 	(low) 	DTR = false (heigh)	OKAY
	// 3) SetRTS(false)	-> RTS = false 	(heigh)	DTR = true 	(low) 	ERROR: DTR toggled
	//
	// In addition this way the CommState Flags are not updated
	/*
		var res bool
		if dtr {
			res = escapeCommFunction(port.handle, commFunctionSetDTR)
		} else {
			res = escapeCommFunction(port.handle, commFunctionClrDTR)
		}
		if !res {
			return &PortError{}
		}
		return nil
	*/

	// The following seems a more reliable way to do it

	params := &dcb{}
	if err := getCommState(port.handle, params); err != nil {
		return &PortError{causedBy: err}
	}
	params.Flags &= dcbDTRControlDisableMask
	if dtr {
		params.Flags |= dcbDTRControlEnable
	}
	if err := setCommState(port.handle, params); err != nil {
		return &PortError{causedBy: err}
	}

	return nil
}

func (port *windowsPort) SetRTS(rts bool) error {
	// It seems that there is a bug in the Windows VCP driver:
	// it doesn't send USB control message when the RTS bit is
	// changed, so the following code not always works with
	// USB-to-serial adapters.
	//
	// In addition this way the CommState Flags are not updated

	/*
		var res bool
		if rts {
			res = escapeCommFunction(port.handle, commFunctionSetRTS)
		} else {
			res = escapeCommFunction(port.handle, commFunctionClrRTS)
		}
		if !res {
			return &PortError{}
		}
		return nil
	*/

	// The following seems a more reliable way to do it

	params := &dcb{}
	if err := getCommState(port.handle, params); err != nil {
		return &PortError{causedBy: err}
	}
	params.Flags &= dcbRTSControlDisableMask
	if rts {
		params.Flags |= dcbRTSControlEnable
	}
	if err := setCommState(port.handle, params); err != nil {
		return &PortError{causedBy: err}
	}
	return nil
}

func (port *windowsPort) GetModemStatusBits() (*ModemStatusBits, error) {
	var bits uint32
	if !getCommModemStatus(port.handle, &bits) {
		return nil, &PortError{}
	}
	return &ModemStatusBits{
		CTS: (bits & msCTSOn) != 0,
		DCD: (bits & msRLSDOn) != 0,
		DSR: (bits & msDSROn) != 0,
		RI:  (bits & msRingOn) != 0,
	}, nil
}

func createOverlappedStruct() (*syscall.Overlapped, error) {
	if h, err := createEvent(nil, true, false, nil); err == nil {
		return &syscall.Overlapped{HEvent: h}, nil
	} else {
		return nil, err
	}
}

func nativeOpen(portName string, mode *Mode) (*windowsPort, error) {
	path, err := syscall.UTF16PtrFromString("\\\\.\\" + portName)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(
		path,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,   // exclusive access
		nil, // no security
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAG_OVERLAPPED,
		0)
	if err != nil {
		switch err {
		case syscall.ERROR_ACCESS_DENIED:
			return nil, &PortError{code: PortBusy}
		case syscall.ERROR_FILE_NOT_FOUND:
			return nil, &PortError{code: PortNotFound}
		}
		return nil, err
	}
	// Create the serial port
	port := &windowsPort{
		handle: handle,
		mode:   mode,
		timeouts: &commTimeouts{
			// Legacy initial timeouts configuration: 1 sec read timeout
			ReadIntervalTimeout:         0xFFFFFFFF,
			ReadTotalTimeoutMultiplier:  0xFFFFFFFF,
			ReadTotalTimeoutConstant:    1000,
			WriteTotalTimeoutMultiplier: 0,
			WriteTotalTimeoutConstant:   0,
		},
	}

	if err = port.reconfigurePort(); err != nil {
		port.Close()
		return nil, err
	}
	return port, nil
}

var parityMap = map[Parity]byte{
	NoParity:    0,
	OddParity:   1,
	EvenParity:  2,
	MarkParity:  3,
	SpaceParity: 4,
}

var stopBitsMap = map[StopBits]byte{
	OneStopBit:           0,
	OnePointFiveStopBits: 1,
	TwoStopBits:          2,
}

func (port *windowsPort) reconfigurePort() error {
	if err := setCommTimeouts(port.handle, port.timeouts); err != nil {
		port.Close()
		return &PortError{code: InvalidSerialPort, causedBy: err}
	}
	if err := setCommMask(port.handle, evErr); err != nil {
		port.Close()
		return &PortError{code: InvalidSerialPort, causedBy: err}
	}
	params := &dcb{}
	if err := getCommState(port.handle, params); err != nil {
		port.Close()
		return &PortError{code: InvalidSerialPort, causedBy: err}
	}
	params.Flags &= dcbRTSControlDisableMask
	params.Flags |= dcbRTSControlEnable
	params.Flags &= dcbDTRControlDisableMask
	params.Flags |= dcbDTRControlEnable
	params.Flags &^= dcbOutXCTSFlow
	params.Flags &^= dcbOutXDSRFlow
	params.Flags &^= dcbDSRSensitivity
	params.Flags |= dcbTXContinueOnXOFF
	params.Flags &^= dcbInX
	params.Flags &^= dcbOutX
	params.Flags &^= dcbErrorChar
	params.Flags &^= dcbNull
	params.Flags &^= dcbAbortOnError
	params.XonLim = 2048
	params.XoffLim = 512
	params.XonChar = 17  // DC1
	params.XoffChar = 19 // C3

	mode := port.mode
	if mode.BaudRate == 0 {
		params.BaudRate = 9600 // Default to 9600
	} else {
		params.BaudRate = uint32(mode.BaudRate)
	}
	if mode.DataBits == 0 {
		params.ByteSize = 8 // Default to 8 bits
	} else {
		params.ByteSize = byte(mode.DataBits)
	}
	params.StopBits = stopBitsMap[mode.StopBits]
	params.Parity = parityMap[mode.Parity]

	if err := setCommState(port.handle, params); err != nil {
		port.Close()
		return &PortError{code: InvalidSerialPort, causedBy: err}
	}
	return nil
}

func (port *windowsPort) SetReadTimeout(t int) error {
	switch {
	case t < 0: // Block until the buffer is full.
		port.timeouts.ReadIntervalTimeout = 0
		port.timeouts.ReadTotalTimeoutMultiplier = 0
		port.timeouts.ReadTotalTimeoutConstant = 0
	case t == 0: // Return immediately with or without data.
		port.timeouts.ReadIntervalTimeout = 0xFFFFFFFF
		port.timeouts.ReadTotalTimeoutMultiplier = 0
		port.timeouts.ReadTotalTimeoutConstant = 0
	case t > 0: // Block until the buffer is full or timeout occurs.
		port.timeouts.ReadIntervalTimeout = 0
		port.timeouts.ReadTotalTimeoutMultiplier = 0
		port.timeouts.ReadTotalTimeoutConstant = uint32(t)
	}
	return port.reconfigurePort()
}

func (port *windowsPort) SetReadTimeoutEx(t, i uint32) error {
	port.timeouts.ReadIntervalTimeout = i
	port.timeouts.ReadTotalTimeoutMultiplier = 0
	port.timeouts.ReadTotalTimeoutConstant = t
	return port.reconfigurePort()
}

func (port *windowsPort) SetLegacyReadTimeout(t uint32) error {
	if t > 0 && t < 0xFFFFFFFF {
		port.timeouts.ReadIntervalTimeout = 0xFFFFFFFF
		port.timeouts.ReadTotalTimeoutMultiplier = 0xFFFFFFFF
		port.timeouts.ReadTotalTimeoutConstant = t
		return port.reconfigurePort()
	} else {
		return &PortError{code: InvalidTimeoutValue}
	}
}

func (port *windowsPort) SetWriteTimeout(t int) error {
	switch {
	case t < 0:
		port.timeouts.WriteTotalTimeoutMultiplier = 0
		port.timeouts.WriteTotalTimeoutConstant = 0
	case t == 0:
		port.timeouts.WriteTotalTimeoutMultiplier = 0
		port.timeouts.WriteTotalTimeoutConstant = 0xFFFFFFFF
	case t > 0:
		port.timeouts.WriteTotalTimeoutMultiplier = 0
		port.timeouts.WriteTotalTimeoutConstant = uint32(t)
	}
	return port.reconfigurePort()
}
