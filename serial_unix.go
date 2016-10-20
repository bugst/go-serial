//
// Copyright 2014-2016 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

// +build linux darwin freebsd

package serial // import "go.bug.st/serial.v1"

import "io/ioutil"
import "regexp"
import "strings"
import "syscall"
import "unsafe"

type unixPort struct {
	handle int
}

func (port *unixPort) Close() error {
	port.releaseExclusiveAccess()
	return syscall.Close(port.handle)
}

func (port *unixPort) Read(p []byte) (n int, err error) {
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
	}

	// Setup serial port
	if port.SetMode(mode) != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}

	// Set raw mode
	settings, err := port.getTermSettings()
	if err != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}
	setRawMode(settings)
	if port.setTermSettings(settings) != nil {
		port.Close()
		return nil, &PortError{code: InvalidSerialPort}
	}

	syscall.SetNonblock(h, false)

	port.acquireExclusiveAccess()

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
	var BAUDMASK uint
	for _, rate := range baudrateMap {
		BAUDMASK |= rate
	}
	settings.Cflag &= ^termiosMask(BAUDMASK)
	// set new baudrate
	settings.Cflag |= termiosMask(baudrate)
	settings.Ispeed = termiosMask(baudrate)
	settings.Ospeed = termiosMask(baudrate)
	return nil
}

func setTermSettingsParity(parity Parity, settings *syscall.Termios) error {
	switch parity {
	case NoParity:
		settings.Cflag &= ^termiosMask(syscall.PARENB | syscall.PARODD | tcCMSPAR)
		settings.Iflag &= ^termiosMask(syscall.INPCK)
	case OddParity:
		settings.Cflag |= termiosMask(syscall.PARENB | syscall.PARODD)
		settings.Cflag &= ^termiosMask(tcCMSPAR)
		settings.Iflag |= termiosMask(syscall.INPCK)
	case EvenParity:
		settings.Cflag &= ^termiosMask(syscall.PARODD | tcCMSPAR)
		settings.Cflag |= termiosMask(syscall.PARENB)
		settings.Iflag |= termiosMask(syscall.INPCK)
	case MarkParity:
		settings.Cflag |= termiosMask(syscall.PARENB | syscall.PARODD | tcCMSPAR)
		settings.Iflag |= termiosMask(syscall.INPCK)
	case SpaceParity:
		settings.Cflag &= ^termiosMask(syscall.PARODD)
		settings.Cflag |= termiosMask(syscall.PARENB | tcCMSPAR)
		settings.Iflag |= termiosMask(syscall.INPCK)
	}
	return nil
}

func setTermSettingsDataBits(bits int, settings *syscall.Termios) error {
	databits, ok := databitsMap[bits]
	if !ok {
		return &PortError{code: InvalidDataBits}
	}
	settings.Cflag &= ^termiosMask(syscall.CSIZE)
	settings.Cflag |= termiosMask(databits)
	return nil
}

func setTermSettingsStopBits(bits StopBits, settings *syscall.Termios) error {
	switch bits {
	case OneStopBit:
		settings.Cflag &= ^termiosMask(syscall.CSTOPB)
	case OnePointFiveStopBits, TwoStopBits:
		settings.Cflag |= termiosMask(syscall.CSTOPB)
	}
	return nil
}

func setRawMode(settings *syscall.Termios) {
	// Set local mode
	settings.Cflag |= termiosMask(syscall.CREAD | syscall.CLOCAL)

	// Explicitly disable RTS/CTS flow control
	settings.Cflag &= ^termiosMask(tcCRTSCTS)

	// Set raw mode
	settings.Lflag &= ^termiosMask(syscall.ICANON | syscall.ECHO | syscall.ECHOE | syscall.ECHOK |
		syscall.ECHONL | syscall.ECHOCTL | syscall.ECHOPRT | syscall.ECHOKE | syscall.ISIG | syscall.IEXTEN)
	settings.Iflag &= ^termiosMask(syscall.IXON | syscall.IXOFF | syscall.IXANY | syscall.INPCK |
		syscall.IGNPAR | syscall.PARMRK | syscall.ISTRIP | syscall.IGNBRK | syscall.BRKINT | syscall.INLCR |
		syscall.IGNCR | syscall.ICRNL | tcIUCLC)
	settings.Oflag &= ^termiosMask(syscall.OPOST)

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

func (port *unixPort) acquireExclusiveAccess() error {
	return ioctl(port.handle, syscall.TIOCEXCL, 0)
}

func (port *unixPort) releaseExclusiveAccess() error {
	return ioctl(port.handle, syscall.TIOCNXCL, 0)
}
