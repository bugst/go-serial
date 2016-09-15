//
// Copyright 2014-2016 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

// +build linux darwin freebsd

package serial // import "go.bug.st/serial"

import "io/ioutil"
import "regexp"
import "strings"
import "syscall"
import "unsafe"

// Opaque type that implements SerialPort interface for linux
type SerialPort struct {
	handle int
}

// Close the serial port
func (port *SerialPort) Close() error {
	port.releaseExclusiveAccess()
	return syscall.Close(port.handle)
}

// Stores data received from the serial port into the provided byte array
// buffer. The function returns the number of bytes read.
//
// The Read function blocks until (at least) one byte is received from
// the serial port or an error occurs.
func (port *SerialPort) Read(p []byte) (n int, err error) {
	return syscall.Read(port.handle, p)
}

// Send the content of the data byte array to the serial port.
// Returns the number of bytes written.
func (port *SerialPort) Write(p []byte) (n int, err error) {
	return syscall.Write(port.handle, p)
}

// Set all parameters of the serial port. See the Mode structure for more
// info.
func (port *SerialPort) SetMode(mode *Mode) error {
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

// Open the serial port using the specified modes
func OpenPort(portName string, mode *Mode) (*SerialPort, error) {
	h, err := syscall.Open(portName, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NDELAY, 0)
	if err != nil {
		switch err {
		case syscall.EBUSY:
			return nil, &SerialPortError{code: ERROR_PORT_BUSY}
		case syscall.EACCES:
			return nil, &SerialPortError{code: ERROR_PERMISSION_DENIED}
		}
		return nil, err
	}
	port := &SerialPort{
		handle: h,
	}

	// Setup serial port
	if port.SetMode(mode) != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}

	// Set raw mode
	settings, err := port.getTermSettings()
	if err != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}
	setRawMode(settings)
	if port.setTermSettings(settings) != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}

	syscall.SetNonblock(h, false)

	port.acquireExclusiveAccess()

	return port, nil
}

func GetPortsList() ([]string, error) {
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
			port, err := OpenPort(portName, &Mode{})
			if err != nil {
				serr, ok := err.(*SerialPortError)
				if ok && serr.Code() == ERROR_INVALID_SERIAL_PORT {
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
		return &SerialPortError{code: ERROR_INVALID_PORT_SPEED}
	}
	// revert old baudrate
	BAUDMASK := 0
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
	case PARITY_NONE:
		settings.Cflag &= ^termiosMask(syscall.PARENB | syscall.PARODD | tc_CMSPAR)
		settings.Iflag &= ^termiosMask(syscall.INPCK)
	case PARITY_ODD:
		settings.Cflag |= termiosMask(syscall.PARENB | syscall.PARODD)
		settings.Cflag &= ^termiosMask(tc_CMSPAR)
		settings.Iflag |= termiosMask(syscall.INPCK)
	case PARITY_EVEN:
		settings.Cflag &= ^termiosMask(syscall.PARODD | tc_CMSPAR)
		settings.Cflag |= termiosMask(syscall.PARENB)
		settings.Iflag |= termiosMask(syscall.INPCK)
	case PARITY_MARK:
		settings.Cflag |= termiosMask(syscall.PARENB | syscall.PARODD | tc_CMSPAR)
		settings.Iflag |= termiosMask(syscall.INPCK)
	case PARITY_SPACE:
		settings.Cflag &= ^termiosMask(syscall.PARODD)
		settings.Cflag |= termiosMask(syscall.PARENB | tc_CMSPAR)
		settings.Iflag |= termiosMask(syscall.INPCK)
	}
	return nil
}

func setTermSettingsDataBits(bits int, settings *syscall.Termios) error {
	databits, ok := databitsMap[bits]
	if !ok {
		return &SerialPortError{code: ERROR_INVALID_PORT_DATA_BITS}
	}
	settings.Cflag &= ^termiosMask(syscall.CSIZE)
	settings.Cflag |= termiosMask(databits)
	return nil
}

func setTermSettingsStopBits(bits StopBits, settings *syscall.Termios) error {
	switch bits {
	case STOPBITS_ONE:
		settings.Cflag &= ^termiosMask(syscall.CSTOPB)
	case STOPBITS_ONEPOINTFIVE, STOPBITS_TWO:
		settings.Cflag |= termiosMask(syscall.CSTOPB)
	}
	return nil
}

func setRawMode(settings *syscall.Termios) {
	// Set local mode
	settings.Cflag |= termiosMask(syscall.CREAD | syscall.CLOCAL)

	// Set raw mode
	settings.Lflag &= ^termiosMask(syscall.ICANON | syscall.ECHO | syscall.ECHOE | syscall.ECHOK |
		syscall.ECHONL | syscall.ECHOCTL | syscall.ECHOPRT | syscall.ECHOKE | syscall.ISIG | syscall.IEXTEN)
	settings.Iflag &= ^termiosMask(syscall.IXON | syscall.IXOFF | syscall.IXANY | syscall.INPCK |
		syscall.IGNPAR | syscall.PARMRK | syscall.ISTRIP | syscall.IGNBRK | syscall.BRKINT | syscall.INLCR |
		syscall.IGNCR | syscall.ICRNL | tc_IUCLC)
	settings.Oflag &= ^termiosMask(syscall.OPOST)

	// Block reads until at least one char is available (no timeout)
	settings.Cc[syscall.VMIN] = 1
	settings.Cc[syscall.VTIME] = 0
}

// native syscall wrapper functions

func (port *SerialPort) getTermSettings() (*syscall.Termios, error) {
	settings := &syscall.Termios{}
	err := ioctl(port.handle, ioctl_tcgetattr, uintptr(unsafe.Pointer(settings)))
	return settings, err
}

func (port *SerialPort) setTermSettings(settings *syscall.Termios) error {
	return ioctl(port.handle, ioctl_tcsetattr, uintptr(unsafe.Pointer(settings)))
}

func (port *SerialPort) acquireExclusiveAccess() error {
	return ioctl(port.handle, syscall.TIOCEXCL, 0)
}

func (port *SerialPort) releaseExclusiveAccess() error {
	return ioctl(port.handle, syscall.TIOCNXCL, 0)
}
