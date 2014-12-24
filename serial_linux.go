//
// Copyright 2014 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

import "io/ioutil"
import "regexp"
import "syscall"
import "unsafe"

// OS dependent values

const devFolder = "/dev"
const regexFilter = "(ttyS|ttyUSB|ttyACM|ttyAMA|rfcomm|ttyO)[0-9]{1,3}"

// opaque type that implements SerialPort interface for linux
type linuxSerialPort struct {
	handle int
}

func (port *linuxSerialPort) Close() error {
	port.releaseExclusiveAccess()
	return syscall.Close(port.handle)
}

func (port *linuxSerialPort) Read(p []byte) (n int, err error) {
	return syscall.Read(port.handle, p)
}

func (port *linuxSerialPort) Write(p []byte) (n int, err error) {
	return syscall.Write(port.handle, p)
}

func (port *linuxSerialPort) SetMode(mode *Mode) error {
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

func OpenPort(portName string, mode *Mode) (SerialPort, error) {
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
	port := &linuxSerialPort{
		handle: h,
	}

	// Setup serial port
	if err := port.SetMode(mode); err != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}

	settings, err := port.getTermSettings()
	if err != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}

	// Set local mode
	settings.Cflag |= syscall.CREAD | syscall.CLOCAL

	// Set raw mode
	settings.Lflag &= ^uint32(syscall.ICANON | syscall.ECHO | syscall.ECHOE | syscall.ECHOK | syscall.ECHONL | syscall.ECHOCTL | syscall.ECHOPRT | syscall.ECHOKE | syscall.ISIG | syscall.IEXTEN)
	settings.Iflag &= ^uint32(syscall.IXON | syscall.IXOFF | syscall.IXANY | syscall.INPCK | syscall.IGNPAR | syscall.PARMRK | syscall.ISTRIP | syscall.IGNBRK | syscall.BRKINT | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IUCLC)
	settings.Oflag &= ^uint32(syscall.OPOST)

	// Block reads until at least one char is available (no timeout)
	settings.Cc[syscall.VMIN] = 1
	settings.Cc[syscall.VTIME] = 0

	err = port.setTermSettings(settings)
	if err != nil {
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
		if f.Name()[:4] == "ttyS" {
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

var baudrateMap = map[int]uint32{
	0:       syscall.B9600, // Default to 9600
	50:      syscall.B50,
	75:      syscall.B75,
	110:     syscall.B110,
	134:     syscall.B134,
	150:     syscall.B150,
	200:     syscall.B200,
	300:     syscall.B300,
	600:     syscall.B600,
	1200:    syscall.B1200,
	1800:    syscall.B1800,
	2400:    syscall.B2400,
	4800:    syscall.B4800,
	9600:    syscall.B9600,
	19200:   syscall.B19200,
	38400:   syscall.B38400,
	57600:   syscall.B57600,
	115200:  syscall.B115200,
	230400:  syscall.B230400,
	460800:  syscall.B460800,
	500000:  syscall.B500000,
	576000:  syscall.B576000,
	921600:  syscall.B921600,
	1000000: syscall.B1000000,
	1152000: syscall.B1152000,
	1500000: syscall.B1500000,
	2000000: syscall.B2000000,
	2500000: syscall.B2500000,
	3000000: syscall.B3000000,
	3500000: syscall.B3500000,
	4000000: syscall.B4000000,
}

var databitsMap = map[int]uint32{
	0: syscall.CS8, // Default to 8 bits
	5: syscall.CS5,
	6: syscall.CS6,
	7: syscall.CS7,
	8: syscall.CS8,
}

func setTermSettingsBaudrate(speed int, settings *syscall.Termios) error {
	baudrate, ok := baudrateMap[speed]
	if !ok {
		return &SerialPortError{code: ERROR_INVALID_PORT_SPEED}
	}
	// revert old baudrate
	var BAUDMASK uint32 = 0
	for _, rate := range baudrateMap {
		BAUDMASK |= rate
	}
	settings.Cflag &= ^uint32(BAUDMASK)
	// set new baudrate
	settings.Cflag |= baudrate
	settings.Ispeed = baudrate
	settings.Ospeed = baudrate
	return nil
}

func setTermSettingsParity(parity Parity, settings *syscall.Termios) error {
	const FIXED_PARITY_FLAG uint32 = 0 // may be CMSPAR or PAREXT
	switch parity {
	case PARITY_NONE:
		settings.Cflag &= ^uint32(syscall.PARENB | syscall.PARODD | FIXED_PARITY_FLAG)
		settings.Iflag &= ^uint32(syscall.INPCK)
	case PARITY_ODD:
		settings.Cflag |= syscall.PARENB | syscall.PARODD
		settings.Cflag &= ^uint32(FIXED_PARITY_FLAG)
		settings.Iflag |= syscall.INPCK
	case PARITY_EVEN:
		settings.Cflag &= ^uint32(syscall.PARODD | FIXED_PARITY_FLAG)
		settings.Cflag |= syscall.PARENB
		settings.Iflag |= syscall.INPCK
	case PARITY_MARK:
		settings.Cflag |= syscall.PARENB | syscall.PARODD | FIXED_PARITY_FLAG
		settings.Iflag |= syscall.INPCK
	case PARITY_SPACE:
		settings.Cflag &= ^uint32(syscall.PARODD)
		settings.Cflag |= syscall.PARENB | FIXED_PARITY_FLAG
		settings.Iflag |= syscall.INPCK
	}
	return nil
}

func setTermSettingsDataBits(bits int, settings *syscall.Termios) error {
	databits, ok := databitsMap[bits]
	if !ok {
		return &SerialPortError{code: ERROR_INVALID_PORT_DATA_BITS}
	}
	settings.Cflag &= ^uint32(syscall.CSIZE)
	settings.Cflag |= databits
	return nil
}

func setTermSettingsStopBits(bits StopBits, settings *syscall.Termios) error {
	switch bits {
	case STOPBITS_ONE:
		settings.Cflag &= ^uint32(syscall.CSTOPB)
	case STOPBITS_ONEPOINTFIVE, STOPBITS_TWO:
		settings.Cflag |= syscall.CSTOPB
	}
	return nil
}

// native syscall wrapper functions

func (port *linuxSerialPort) acquireExclusiveAccess() error {
	return ioctl(port.handle, syscall.TIOCEXCL, 0)
}

func (port *linuxSerialPort) releaseExclusiveAccess() error {
	return ioctl(port.handle, syscall.TIOCNXCL, 0)
}

func (port *linuxSerialPort) getTermSettings() (*syscall.Termios, error) {
	settings := &syscall.Termios{}
	err := ioctl(port.handle, syscall.TCGETS, uintptr(unsafe.Pointer(settings)))
	return settings, err
}

func (port *linuxSerialPort) setTermSettings(settings *syscall.Termios) error {
	return ioctl(port.handle, syscall.TCSETS, uintptr(unsafe.Pointer(settings)))
}

//sys ioctl(fd int, req uint64, data uintptr) (err error)
func ioctl(fd int, req uint64, data uintptr) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(data))
	if e1 != 0 {
		err = e1
	}
	return
}

// vi:ts=2
