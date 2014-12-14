//
// Copyright 2014 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

/*

#include <fcntl.h>
#include <errno.h>
#include <stdio.h>
#include <sys/ioctl.h>
#include <sys/select.h>
#include <termios.h>
#include <time.h>
#include <unistd.h>

#include <linux/serial.h>

// Define (eventually) missing constants
#ifndef IUCLC
	const tcflag_t IUCLC = 0;
#endif

#if defined(PAREXT)
	const tcflag_t FIXED_PAR_FLAG = PAREXT;
#elif defined(CMSPAR)
	const tcflag_t FIXED_PAR_FLAG = CMSPAR;
#else
	const tcflag_t FIXED_PAR_FLAG = 0;
#endif

// ioctl call is not available through syscall package
//int ioctl_wrapper(int d, unsigned long request) {
//	return ioctl(d, request);
//}

//int fcntl_wrapper(int fd, int cmd, int arg) {
//	return fcntl(fd, cmd, arg);
//}

// Gain exclusive access to serial port
void setTIOCEXCL(int handle) {
#if defined TIOCEXCL
	ioctl(handle, TIOCEXCL);
#endif
}

// Release exclusive access to serial port
void setTIOCNXCL(int handle) {
#if defined TIOCNXCL
	ioctl(handle, TIOCNXCL);
#endif
}

//int selectRead(int handle) {
//	fd_set rfds;
//	FD_ZERO(&rfds);
//	FD_SET(handle, &rfds);
//  int ret = select(handle+1, &rfds, NULL, NULL, NULL);
//	if (ret==-1)
//		return -1;
//	else
//		return 0;
//}

*/
import "C"
import "io/ioutil"
import "regexp"
import "syscall"

// native syscall wrapper functions

func getExclusiveAccess(handle int) error {
	_, err := C.setTIOCEXCL(C.int(handle))
	return err
}

func releaseExclusiveAccess(handle int) error {
	_, err := C.setTIOCNXCL(C.int(handle))
	return err
}

func getTermSettings(handle int) (*C.struct_termios, error) {
	settings := new(C.struct_termios)
	_, err := C.tcgetattr(C.int(handle), settings)
	return settings, err
}

func setTermSettings(handle int, settings *C.struct_termios) error {
	_, err := C.tcsetattr(C.int(handle), C.TCSANOW, settings)
	return err
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

	ports := make([]string, len(files))
	found := 0
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

		// Save found serial port in the resulting list
		ports[found] = portName
		found++
	}

	ports = ports[:found]
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

var baudrateMap = map[int]C.speed_t{
	0:       C.B0,
	50:      C.B50,
	75:      C.B75,
	110:     C.B110,
	134:     C.B134,
	150:     C.B150,
	200:     C.B200,
	300:     C.B300,
	600:     C.B600,
	1200:    C.B1200,
	1800:    C.B1800,
	2400:    C.B2400,
	4800:    C.B4800,
	9600:    C.B9600,
	19200:   C.B19200,
	38400:   C.B38400,
	57600:   C.B57600,
	115200:  C.B115200,
	230400:  C.B230400,
	460800:  C.B460800,
	500000:  C.B500000,
	576000:  C.B576000,
	921600:  C.B921600,
	1000000: C.B1000000,
	1152000: C.B1152000,
	1500000: C.B1500000,
	2000000: C.B2000000,
	2500000: C.B2500000,
	3000000: C.B3000000,
	3500000: C.B3500000,
	4000000: C.B4000000,
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
	C.cfsetispeed(settings, baudrate)
	C.cfsetospeed(settings, baudrate)
	return setTermSettings(port.Handle, settings)
}

func (port *linuxSerialPort) SetParity(parity Parity) error {
	settings, err := getTermSettings(port.Handle)
	if err != nil {
		return err
	}
	switch parity {
	case PARITY_NONE:
		settings.c_cflag &= ^C.tcflag_t(syscall.PARENB | syscall.PARODD | C.FIXED_PAR_FLAG)
		settings.c_iflag &= ^C.tcflag_t(syscall.INPCK)
	case PARITY_ODD:
		settings.c_cflag |= syscall.PARENB | syscall.PARODD
		settings.c_cflag &= ^C.tcflag_t(C.FIXED_PAR_FLAG)
		settings.c_iflag |= syscall.INPCK
	case PARITY_EVEN:
		settings.c_cflag &= ^C.tcflag_t(syscall.PARODD | C.FIXED_PAR_FLAG)
		settings.c_cflag |= syscall.PARENB
		settings.c_iflag |= syscall.INPCK
	case PARITY_MARK:
		settings.c_cflag |= syscall.PARENB | syscall.PARODD | C.FIXED_PAR_FLAG
		settings.c_iflag |= syscall.INPCK
	case PARITY_SPACE:
		settings.c_cflag &= ^C.tcflag_t(syscall.PARODD)
		settings.c_cflag |= syscall.PARENB | C.FIXED_PAR_FLAG
		settings.c_iflag |= syscall.INPCK
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
	settings.c_cflag |= C.CREAD | C.CLOCAL

	// Set raw mode
	settings.c_lflag &= ^C.tcflag_t(C.ICANON | C.ECHO | C.ECHOE | C.ECHOK | C.ECHONL | C.ECHOCTL | C.ECHOPRT | C.ECHOKE | C.ISIG | C.IEXTEN)
	settings.c_iflag &= ^C.tcflag_t(C.IXON | C.IXOFF | C.IXANY | C.INPCK | C.IGNPAR | C.PARMRK | C.ISTRIP | C.IGNBRK | C.BRKINT | C.INLCR | C.IGNCR | C.ICRNL | C.IUCLC)
	settings.c_oflag &= ^C.tcflag_t(C.OPOST)

	// Block reads until at least one char is available (no timeout)
	settings.c_cc[C.VMIN] = 1
	settings.c_cc[C.VTIME] = 0

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
