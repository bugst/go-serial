//go:build linux

package serial

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// GetSerialStruct opens the device and retrieves the CSerialStruct using TIOCGSERIAL ioctl.
func (port *unixPort) GetSerialStruct() (*LinuxCSerialStruct, error) {
	var ser LinuxCSerialStruct
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(port.handle), unix.TIOCGSERIAL, uintptr(unsafe.Pointer(&ser)))
	if errno != 0 {
		return nil, fmt.Errorf("ioctl TIOCGSERIAL failed: %v", errno)
	}
	return &ser, nil
}

// SetSerialPortMode sets the port mode using ioctl TIOCSSERIAL
func (port *unixPort) SetSerialPortMode(portMode uint32) error {
	ser, err := port.GetSerialStruct()
	if err != nil {
		return err
	}

	ser.Port = portMode
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(port.handle), unix.TIOCSSERIAL, uintptr(unsafe.Pointer(ser)))
	if errno != 0 {
		return fmt.Errorf("ioctl TIOCSSERIAL failed: %v", errno)
	}
	return nil
}
