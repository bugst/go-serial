//
// Copyright 2014-2016 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

// +build linux darwin freebsd

package serial // import "go.bug.st/serial.v1"

import (
	"io/ioutil"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"go.bug.st/serial.v1/unixutils"
)

type unixPort struct {
	handle int
	name   string

	closeLock   sync.RWMutex
	closeSignal *unixutils.Pipe
	opened      bool
}

func (port *unixPort) GetName() string {
	return port.name
}

func (port *unixPort) Close() error {
	// Close port
	port.releaseExclusiveAccess()
	if err := syscall.Close(port.handle); err != nil {
		return err
	}
	port.opened = false

	if port.closeSignal != nil {
		// Send close signal to all pending reads (if any)
		port.closeSignal.Write([]byte{0})

		// Wait for all readers to complete
		port.closeLock.Lock()
		defer port.closeLock.Unlock()

		// Close signaling pipe
		if err := port.closeSignal.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (port *unixPort) Read(p []byte) (n int, err error) {
	port.closeLock.RLock()
	defer port.closeLock.RUnlock()
	if !port.opened {
		return 0, &PortError{code: PortClosed}
	}

	fds := unixutils.NewFDSet(port.handle, port.closeSignal.ReadFD())
	res, err := unixutils.Select(fds, nil, fds, -1)
	if err != nil {
		return 0, err
	}
	if res.IsReadable(port.closeSignal.ReadFD()) {
		return 0, &PortError{code: PortClosed}
	}
	return syscall.Read(port.handle, p)
}

func (port *unixPort) Write(p []byte) (n int, err error) {
	return syscall.Write(port.handle, p)
}

func (port *unixPort) SetMode(mode *Mode) error {
	settings, err := port.getTermSettings()
	if err != nil {
		return err
	}
	if err := setTermSettingsBaudrate(mode.BaudRate, settings); err != nil {
		return err
	}
	if err := setTermSettingsParity(mode.Parity, settings); err != nil {
		return err
	}
	if err := setTermSettingsDataBits(mode.DataBits, settings); err != nil {
		return err
	}
	if err := setTermSettingsStopBits(mode.StopBits, settings); err != nil {
		return err
	}
	return port.setTermSettings(settings)
}

func (port *unixPort) SetDTR(dtr bool) error {
	status, err := port.getModemBitsStatus()
	if err != nil {
		return err
	}
	if dtr {
		status |= syscall.TIOCM_DTR
	} else {
		status &^= syscall.TIOCM_DTR
	}
	return port.setModemBitsStatus(status)
}

func (port *unixPort) SetRTS(rts bool) error {
	status, err := port.getModemBitsStatus()
	if err != nil {
		return err
	}
	if rts {
		status |= syscall.TIOCM_RTS
	} else {
		status &^= syscall.TIOCM_RTS
	}
	return port.setModemBitsStatus(status)
}

func (port *unixPort) GetModemStatusBits() (*ModemStatusBits, error) {
	status, err := port.getModemBitsStatus()
	if err != nil {
		return nil, err
	}
	return &ModemStatusBits{
		CTS: (status & syscall.TIOCM_CTS) != 0,
		DCD: (status & syscall.TIOCM_CD) != 0,
		DSR: (status & syscall.TIOCM_DSR) != 0,
		RI:  (status & syscall.TIOCM_RI) != 0,
	}, nil
}

func nativeOpen(portName string, mode *Mode) (*unixPort, error) {
	h, err := syscall.Open(portName, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NDELAY, 0)
	if err != nil {
		switch err {
		case syscall.EBUSY:
			return nil, &PortError{code: PortBusy}
		case syscall.EACCES:
			return nil, &PortError{code: PermissionDenied}
		}
		return nil, err
	}
	port := &unixPort{
		handle: h,
		name:   portName,
		opened: true,
	}

	// Setup serial port
	if port.SetMode(mode) != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}

	settings, err := port.getTermSettings()
	if err != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}

	// Set raw mode
	setRawMode(settings)

	// Explicitly disable RTS/CTS flow control
	setTermSettingsCtsRts(false, settings)

	if port.setTermSettings(settings) != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}

	syscall.SetNonblock(h, false)

	port.acquireExclusiveAccess()

	// This pipe is used as a signal to cancel blocking Read
	pipe := &unixutils.Pipe{}
	if err := pipe.Open(); err != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort, causedBy: err}
	}
	port.closeSignal = pipe

	return port, nil
}

func nativeGetPortsList() ([]string, error) {
	files, err := ioutil.ReadDir(devFolder)
	if err != nil {
		return nil, err
	}

	ports := make([]string, 0, len(files))
	for _, f := range files {
		// Skip folders
		if f.IsDir() {
			continue
		}

		// Keep only devices with the correct name
		match, err := regexp.MatchString(regexFilter, f.Name())
		if err != nil {
			return nil, err
		}
		if !match {
			continue
		}

		portName := devFolder + "/" + f.Name()

		// Check if serial port is real or is a placeholder serial port "ttySxx"
		if strings.HasPrefix(f.Name(), "ttyS") {
			port, err := nativeOpen(portName, &Mode{})
			if err != nil {
				serr, ok := err.(*PortError)
				if ok && serr.Code() == InvalidSerialPort {
					continue
				}
			} else {
				port.Close()
			}
		}

		// Save serial port in the resulting list
		ports = append(ports, portName)
	}

	return ports, nil
}

// termios manipulation functions

func setTermSettingsBaudrate(speed int, settings *syscall.Termios) error {
	baudrate, ok := baudrateMap[speed]
	if !ok {
		return &PortError{code: InvalidSpeed}
	}
	// revert old baudrate
	for _, rate := range baudrateMap {
		settings.Cflag &^= rate
	}
	// set new baudrate
	settings.Cflag |= baudrate
	settings.Ispeed = baudrate
	settings.Ospeed = baudrate
	return nil
}

func setTermSettingsParity(parity Parity, settings *syscall.Termios) error {
	switch parity {
	case NoParity:
		settings.Cflag &^= syscall.PARENB
		settings.Cflag &^= syscall.PARODD
		settings.Cflag &^= tcCMSPAR
		settings.Iflag &^= syscall.INPCK
	case OddParity:
		settings.Cflag |= syscall.PARENB
		settings.Cflag |= syscall.PARODD
		settings.Cflag &^= tcCMSPAR
		settings.Iflag |= syscall.INPCK
	case EvenParity:
		settings.Cflag |= syscall.PARENB
		settings.Cflag &^= syscall.PARODD
		settings.Cflag &^= tcCMSPAR
		settings.Iflag |= syscall.INPCK
	case MarkParity:
		if tcCMSPAR == 0 {
			return &PortError{code: InvalidParity}
		}
		settings.Cflag |= syscall.PARENB
		settings.Cflag |= syscall.PARODD
		settings.Cflag |= tcCMSPAR
		settings.Iflag |= syscall.INPCK
	case SpaceParity:
		if tcCMSPAR == 0 {
			return &PortError{code: InvalidParity}
		}
		settings.Cflag |= syscall.PARENB
		settings.Cflag &^= syscall.PARODD
		settings.Cflag |= tcCMSPAR
		settings.Iflag |= syscall.INPCK
	default:
		return &PortError{code: InvalidParity}
	}
	return nil
}

func setTermSettingsDataBits(bits int, settings *syscall.Termios) error {
	databits, ok := databitsMap[bits]
	if !ok {
		return &PortError{code: InvalidDataBits}
	}
	// Remove previous databits setting
	settings.Cflag &^= syscall.CSIZE
	// Set requested databits
	settings.Cflag |= databits
	return nil
}

func setTermSettingsStopBits(bits StopBits, settings *syscall.Termios) error {
	switch bits {
	case OneStopBit:
		settings.Cflag &^= syscall.CSTOPB
	case OnePointFiveStopBits:
		return &PortError{code: InvalidStopBits}
	case TwoStopBits:
		settings.Cflag |= syscall.CSTOPB
	default:
		return &PortError{code: InvalidStopBits}
	}
	return nil
}

func setTermSettingsCtsRts(enable bool, settings *syscall.Termios) {
	if enable {
		settings.Cflag |= tcCRTSCTS
	} else {
		settings.Cflag &^= tcCRTSCTS
	}
}

func setRawMode(settings *syscall.Termios) {
	// Set local mode
	settings.Cflag |= syscall.CREAD
	settings.Cflag |= syscall.CLOCAL

	// Set raw mode
	settings.Lflag &^= syscall.ICANON
	settings.Lflag &^= syscall.ECHO
	settings.Lflag &^= syscall.ECHOE
	settings.Lflag &^= syscall.ECHOK
	settings.Lflag &^= syscall.ECHONL
	settings.Lflag &^= syscall.ECHOCTL
	settings.Lflag &^= syscall.ECHOPRT
	settings.Lflag &^= syscall.ECHOKE
	settings.Lflag &^= syscall.ISIG
	settings.Lflag &^= syscall.IEXTEN

	settings.Iflag &^= syscall.IXON
	settings.Iflag &^= syscall.IXOFF
	settings.Iflag &^= syscall.IXANY
	settings.Iflag &^= syscall.INPCK
	settings.Iflag &^= syscall.IGNPAR
	settings.Iflag &^= syscall.PARMRK
	settings.Iflag &^= syscall.ISTRIP
	settings.Iflag &^= syscall.IGNBRK
	settings.Iflag &^= syscall.BRKINT
	settings.Iflag &^= syscall.INLCR
	settings.Iflag &^= syscall.IGNCR
	settings.Iflag &^= syscall.ICRNL
	settings.Iflag &^= tcIUCLC

	settings.Oflag &^= syscall.OPOST

	// Block reads until at least one char is available (no timeout)
	settings.Cc[syscall.VMIN] = 1
	settings.Cc[syscall.VTIME] = 0
}

// native syscall wrapper functions

func (port *unixPort) getTermSettings() (*syscall.Termios, error) {
	settings := &syscall.Termios{}
	err := ioctl(port.handle, ioctlTcgetattr, uintptr(unsafe.Pointer(settings)))
	return settings, err
}

func (port *unixPort) setTermSettings(settings *syscall.Termios) error {
	return ioctl(port.handle, ioctlTcsetattr, uintptr(unsafe.Pointer(settings)))
}

func (port *unixPort) getModemBitsStatus() (int, error) {
	var status int
	err := ioctl(port.handle, syscall.TIOCMGET, uintptr(unsafe.Pointer(&status)))
	return status, err
}

func (port *unixPort) setModemBitsStatus(status int) error {
	return ioctl(port.handle, syscall.TIOCMSET, uintptr(unsafe.Pointer(&status)))
}

func (port *unixPort) acquireExclusiveAccess() error {
	return ioctl(port.handle, syscall.TIOCEXCL, 0)
}

func (port *unixPort) releaseExclusiveAccess() error {
	return ioctl(port.handle, syscall.TIOCNXCL, 0)
}
