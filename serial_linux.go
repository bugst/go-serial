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

//sys ioctl(fd int, req uint64, data uintptr) (err error)
func ioctl(fd int, req uint64, data uintptr) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(data))
	if e1 != 0 {
		err = e1
	}
	return
}

// native syscall wrapper functions

func getExclusiveAccess(handle int) error {
	return ioctl(handle, syscall.TIOCEXCL, 0)
}

func releaseExclusiveAccess(handle int) error {
	return ioctl(handle, syscall.TIOCNXCL, 0)
}

func getTermSettings(handle int) (*syscall.Termios, error) {
	settings := &syscall.Termios{}
	err := ioctl(handle, syscall.TCGETS, uintptr(unsafe.Pointer(settings)))
	return settings, err
}

func setTermSettings(handle int, settings *syscall.Termios) error {
	return ioctl(handle, syscall.TCSETS, uintptr(unsafe.Pointer(settings)))
}

func getErrno(err error) int {
	return int(err.(syscall.Errno))
}

// OS dependent values

const devFolder = "/dev"
const regexFilter = "(ttyS|ttyUSB|ttyACM|ttyAMA|rfcomm|ttyO)[0-9]{1,3}"

// opaque type that implements SerialPort interface for linux
type linuxSerialPort struct {
	Handle int
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
			port, err := OpenPort(portName, false)
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

func (port *linuxSerialPort) Close() error {
	releaseExclusiveAccess(port.Handle)
	return syscall.Close(port.Handle)
}

func (port *linuxSerialPort) Read(p []byte) (n int, err error) {
	return syscall.Read(port.Handle, p)
}

func (port *linuxSerialPort) Write(p []byte) (n int, err error) {
	return syscall.Write(port.Handle, p)
}

var baudrateMap = map[int]uint32{
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

func (port *linuxSerialPort) SetSpeed(speed int) error {
	baudrate, ok := baudrateMap[speed]
	if !ok {
		return &SerialPortError{code: ERROR_INVALID_PORT_SPEED}
	}
	settings, err := getTermSettings(port.Handle)
	if err != nil {
		return err
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
	return setTermSettings(port.Handle, settings)
}

func (port *linuxSerialPort) SetParity(parity Parity) error {
	const FIXED_PARITY_FLAG uint32 = 0 // may be CMSPAR or PAREXT

	settings, err := getTermSettings(port.Handle)
	if err != nil {
		return err
	}
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
	return setTermSettings(port.Handle, settings)
}

var databitsMap = map[int]uint32{
	5: syscall.CS5,
	6: syscall.CS6,
	7: syscall.CS7,
	8: syscall.CS8,
}

func (port *linuxSerialPort) SetDataBits(bits int) error {
	databits, ok := databitsMap[bits]
	if !ok {
		return &SerialPortError{code: ERROR_INVALID_PORT_DATA_BITS}
	}
	settings, err := getTermSettings(port.Handle)
	if err != nil {
		return err
	}
	settings.Cflag &= ^uint32(syscall.CSIZE)
	settings.Cflag |= databits
	return setTermSettings(port.Handle, settings)
}

func (port *linuxSerialPort) SetStopBits(bits StopBits) error {
	settings, err := getTermSettings(port.Handle)
	if err != nil {
		return err
	}
	switch bits {
	case STOPBITS_ONE:
		settings.Cflag &= ^uint32(syscall.CSTOPB)
	case STOPBITS_ONEPOINTFIVE, STOPBITS_TWO:
		settings.Cflag |= syscall.CSTOPB
	}
	return setTermSettings(port.Handle, settings)
}

func OpenPort(portName string, exclusive bool) (SerialPort, error) {
	handle, err := syscall.Open(portName, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NDELAY, 0)
	if err != nil {
		switch err {
		case syscall.EBUSY:
			return nil, &SerialPortError{code: ERROR_PORT_BUSY}
		case syscall.EACCES:
			return nil, &SerialPortError{code: ERROR_PERMISSION_DENIED}
		}
		return nil, err
	}

	// Setup serial port with defaults

	settings, err := getTermSettings(handle)
	if err != nil {
		syscall.Close(handle)
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

	err = setTermSettings(handle, settings)
	if err != nil {
		syscall.Close(handle)
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}
	/*
	   settings->c_cflag &= ~CRTSCTS;
	*/
	syscall.SetNonblock(handle, false)

	if exclusive {
		getExclusiveAccess(handle)
	}

	serialPort := &linuxSerialPort{
		Handle: handle,
	}
	return serialPort, nil
}

// vi:ts=2
