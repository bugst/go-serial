//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

/*

// MSDN article on Serial Communications:
// http://msdn.microsoft.com/en-us/library/ff802693.aspx
// (alternative link) https://msdn.microsoft.com/en-us/library/ms810467.aspx

// Arduino Playground article on serial communication with Windows API:
// http://playground.arduino.cc/Interfacing/CPPWindows

*/

import (
	"errors"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type windowsPort struct {
	mu         sync.Mutex
	handle     windows.Handle
	hasTimeout bool
}

func nativeGetPortsList() ([]string, error) {
	key, err := registry.OpenKey(windows.HKEY_LOCAL_MACHINE, `HARDWARE\DEVICEMAP\SERIALCOMM\`, windows.KEY_READ)
	switch {
	case errors.Is(err, syscall.ERROR_FILE_NOT_FOUND):
		// On machines with no serial ports the registry key does not exist.
		// Return this as no serial ports instead of an error.
		return nil, nil
	case err != nil:
		return nil, &PortError{code: ErrorEnumeratingPorts, causedBy: err}
	}
	defer key.Close()

	list, err := key.ReadValueNames(0)
	if err != nil {
		return nil, &PortError{code: ErrorEnumeratingPorts, causedBy: err}
	}

	return list, nil
}

func (port *windowsPort) Close() error {
	port.mu.Lock()
	defer func() {
		port.handle = 0
		port.mu.Unlock()
	}()
	if port.handle == 0 {
		return nil
	}
	return windows.CloseHandle(port.handle)
}

func (port *windowsPort) Read(p []byte) (int, error) {
	var readed uint32
	ev, err := createOverlappedEvent()
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(ev.HEvent)

	for {
		err = windows.ReadFile(port.handle, p, &readed, ev)
		if err == windows.ERROR_IO_PENDING {
			err = windows.GetOverlappedResult(port.handle, ev, &readed, true)
		}
		switch err {
		case nil:
			// operation completed successfully
		case windows.ERROR_OPERATION_ABORTED:
			// port may have been closed
			return int(readed), &PortError{code: PortClosed, causedBy: err}
		default:
			// error happened
			return int(readed), err
		}
		if readed > 0 {
			return int(readed), nil
		}

		// Timeout
		port.mu.Lock()
		hasTimeout := port.hasTimeout
		port.mu.Unlock()
		if hasTimeout {
			return 0, nil
		}
	}
}

func (port *windowsPort) Write(p []byte) (int, error) {
	var writed uint32
	ev, err := createOverlappedEvent()
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(ev.HEvent)
	err = windows.WriteFile(port.handle, p, &writed, ev)
	if err == windows.ERROR_IO_PENDING {
		// wait for write to complete
		err = windows.GetOverlappedResult(port.handle, ev, &writed, true)
	}
	return int(writed), err
}

func (port *windowsPort) Drain() (err error) {
	return windows.FlushFileBuffers(port.handle)
}

func (port *windowsPort) ResetInputBuffer() error {
	return windows.PurgeComm(port.handle, windows.PURGE_RXCLEAR|windows.PURGE_RXABORT)
}

func (port *windowsPort) ResetOutputBuffer() error {
	return windows.PurgeComm(port.handle, windows.PURGE_TXCLEAR|windows.PURGE_TXABORT)
}

const (
	dcbBinary                uint32 = 0x00000001
	dcbParity                       = 0x00000002
	dcbOutXCTSFlow                  = 0x00000004
	dcbOutXDSRFlow                  = 0x00000008
	dcbDTRControlDisableMask        = ^uint32(0x00000030)
	dcbDTRControlEnable             = 0x00000010
	dcbDTRControlHandshake          = 0x00000020
	dcbDSRSensitivity               = 0x00000040
	dcbTXContinueOnXOFF             = 0x00000080
	dcbOutX                         = 0x00000100
	dcbInX                          = 0x00000200
	dcbErrorChar                    = 0x00000400
	dcbNull                         = 0x00000800
	dcbRTSControlDisableMask        = ^uint32(0x00003000)
	dcbRTSControlEnable             = 0x00001000
	dcbRTSControlHandshake          = 0x00002000
	dcbRTSControlToggle             = 0x00003000
	dcbAbortOnError                 = 0x00004000
)

var parityMap = map[Parity]byte{
	NoParity:    windows.NOPARITY,
	OddParity:   windows.ODDPARITY,
	EvenParity:  windows.EVENPARITY,
	MarkParity:  windows.MARKPARITY,
	SpaceParity: windows.SPACEPARITY,
}

var stopBitsMap = map[StopBits]byte{
	OneStopBit:           windows.ONESTOPBIT,
	OnePointFiveStopBits: windows.ONE5STOPBITS,
	TwoStopBits:          windows.TWOSTOPBITS,
}

func (port *windowsPort) SetMode(mode *Mode) error {
	params := windows.DCB{}
	if windows.GetCommState(port.handle, &params) != nil {
		port.Close()
		return &PortError{code: InvalidSerialPort}
	}
	port.setModeParams(mode, &params)
	if windows.SetCommState(port.handle, &params) != nil {
		port.Close()
		return &PortError{code: InvalidSerialPort}
	}
	return nil
}

func (port *windowsPort) setModeParams(mode *Mode, params *windows.DCB) {
	if mode.BaudRate == 0 {
		params.BaudRate = windows.CBR_9600 // Default to 9600
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
}

func (port *windowsPort) SetDTR(dtr bool) error {
	// Like for RTS there are problems with the windows.EscapeCommFunction
	// observed behaviour was that DTR is set from false -> true
	// when setting RTS from true -> false
	// 1) Connect 		-> RTS = true 	(low) 	DTR = true 	(low) 	OKAY
	// 2) SetDTR(false) -> RTS = true 	(low) 	DTR = false (high)	OKAY
	// 3) SetRTS(false)	-> RTS = false 	(high)	DTR = true 	(low) 	ERROR: DTR toggled
	//
	// In addition this way the CommState Flags are not updated
	/*
		var err error
		if dtr {
			err = windows.EscapeCommFunction(port.handle, windows.SETDTR)
		} else {
			err = windows.EscapeCommFunction(port.handle, windows.CLTDTR)
		}
		if err != nil {
			return &PortError{}
		}
		return nil
	*/

	// The following seems a more reliable way to do it

	params := &windows.DCB{}
	if err := windows.GetCommState(port.handle, params); err != nil {
		return &PortError{causedBy: err}
	}
	params.Flags &= dcbDTRControlDisableMask
	if dtr {
		params.Flags |= windows.DTR_CONTROL_ENABLE
	}
	if err := windows.SetCommState(port.handle, params); err != nil {
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
		var err error
		if rts {
			err = windows.EscapeCommFunction(port.handle, windows.SETRTS)
		} else {
			err = windows.EscapeCommFunction(port.handle, windows.CLRRTS)
		}
		if err != nil {
			return &PortError{}
		}
		return nil
	*/

	// The following seems a more reliable way to do it

	params := &windows.DCB{}
	if err := windows.GetCommState(port.handle, params); err != nil {
		return &PortError{causedBy: err}
	}
	params.Flags &= dcbRTSControlDisableMask
	if rts {
		params.Flags |= windows.RTS_CONTROL_ENABLE
	}
	if err := windows.SetCommState(port.handle, params); err != nil {
		return &PortError{causedBy: err}
	}
	return nil
}

func (port *windowsPort) GetModemStatusBits() (*ModemStatusBits, error) {
	var bits uint32
	if err := windows.GetCommModemStatus(port.handle, &bits); err != nil {
		return nil, &PortError{}
	}
	return &ModemStatusBits{
		CTS: (bits & windows.EV_CTS) != 0,
		DCD: (bits & windows.EV_RLSD) != 0,
		DSR: (bits & windows.EV_DSR) != 0,
		RI:  (bits & windows.EV_RING) != 0,
	}, nil
}

func (port *windowsPort) SetReadTimeout(timeout time.Duration) error {
	// This is a brutal hack to make the CH340 chipset work properly.
	// Normally this value should be 0xFFFFFFFE but, after a lot of
	// tinkering, I discovered that any value with the highest
	// bit set will make the CH340 driver behave like the timeout is 0,
	// in the best cases leading to a spinning loop...
	// (could this be a wrong signed vs unsigned conversion in the driver?)
	// https://github.com/arduino/serial-monitor/issues/112
	const MaxReadTotalTimeoutConstant = 0x7FFFFFFE

	commTimeouts := &windows.CommTimeouts{
		ReadIntervalTimeout:         0xFFFFFFFF,
		ReadTotalTimeoutMultiplier:  0xFFFFFFFF,
		ReadTotalTimeoutConstant:    MaxReadTotalTimeoutConstant,
		WriteTotalTimeoutConstant:   0,
		WriteTotalTimeoutMultiplier: 0,
	}
	if timeout != NoTimeout {
		ms := timeout.Milliseconds()
		if ms > 0xFFFFFFFE || ms < 0 {
			return &PortError{code: InvalidTimeoutValue}
		}

		if ms > MaxReadTotalTimeoutConstant {
			ms = MaxReadTotalTimeoutConstant
		}

		commTimeouts.ReadTotalTimeoutConstant = uint32(ms)
	}

	port.mu.Lock()
	defer port.mu.Unlock()
	if err := windows.SetCommTimeouts(port.handle, commTimeouts); err != nil {
		return &PortError{code: InvalidTimeoutValue, causedBy: err}
	}
	port.hasTimeout = (timeout != NoTimeout)

	return nil
}

func (port *windowsPort) Break(d time.Duration) error {
	if err := windows.SetCommBreak(port.handle); err != nil {
		return &PortError{causedBy: err}
	}

	time.Sleep(d)

	if err := windows.ClearCommBreak(port.handle); err != nil {
		return &PortError{causedBy: err}
	}

	return nil
}

func createOverlappedEvent() (*windows.Overlapped, error) {
	h, err := windows.CreateEvent(nil, 1, 0, nil)
	return &windows.Overlapped{HEvent: h}, err
}

func nativeOpen(portName string, mode *Mode) (*windowsPort, error) {
	portName = "\\\\.\\" + portName
	path, err := windows.UTF16PtrFromString(portName)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		path,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED,
		0)
	if err != nil {
		switch err {
		case windows.ERROR_ACCESS_DENIED:
			return nil, &PortError{code: PortBusy}
		case windows.ERROR_FILE_NOT_FOUND:
			return nil, &PortError{code: PortNotFound}
		}
		return nil, err
	}
	// Create the serial port
	port := &windowsPort{
		handle: handle,
	}

	// Set port parameters
	params := &windows.DCB{}
	if windows.GetCommState(port.handle, params) != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}
	port.setModeParams(mode, params)
	params.Flags &= dcbDTRControlDisableMask
	params.Flags &= dcbRTSControlDisableMask
	if mode.InitialStatusBits == nil {
		params.Flags |= windows.DTR_CONTROL_ENABLE
		params.Flags |= windows.RTS_CONTROL_ENABLE
	} else {
		if mode.InitialStatusBits.DTR {
			params.Flags |= windows.DTR_CONTROL_ENABLE
		}
		if mode.InitialStatusBits.RTS {
			params.Flags |= windows.RTS_CONTROL_ENABLE
		}
	}

	if mode.RTSCTSFlowControl {
		params.Flags |= dcbOutXCTSFlow
	} else {
		params.Flags &^= dcbOutXCTSFlow
	}

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
	if windows.SetCommState(port.handle, params) != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}

	if port.SetReadTimeout(NoTimeout) != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}
	return port, nil
}
