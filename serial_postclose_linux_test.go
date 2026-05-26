//
// Copyright 2014-2026 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

//go:build linux

package serial

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// openPTYPort opens a Linux pseudo-terminal and returns a serial Port bound to
// its slave side. No real serial hardware is required.
func openPTYPort(t *testing.T) Port {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("cannot open /dev/ptmx: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	mfd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(mfd, unix.TIOCSPTLCK, 0); err != nil {
		t.Skipf("TIOCSPTLCK: %v", err)
	}
	n, err := unix.IoctlGetInt(mfd, unix.TIOCGPTN)
	if err != nil {
		t.Skipf("TIOCGPTN: %v", err)
	}
	slave := fmt.Sprintf("/dev/pts/%d", n)
	port, err := Open(slave, &Mode{BaudRate: 9600})
	if err != nil {
		t.Skipf("cannot open pty slave %s: %v", slave, err)
	}
	return port
}

// TestOperationsAfterCloseReturnPortClosed verifies that every method that
// touches the file descriptor returns PortClosed after the port is closed,
// instead of operating on a stale fd whose number the OS may have reassigned.
func TestOperationsAfterCloseReturnPortClosed(t *testing.T) {
	port := openPTYPort(t)
	if err := port.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	assertClosed := func(name string, err error) {
		t.Helper()
		var pe *PortError
		if !errors.As(err, &pe) || pe.Code() != PortClosed {
			t.Errorf("%s after Close: got %v, want PortError{PortClosed}", name, err)
		}
	}

	n, err := port.Write([]byte("data"))
	if n != 0 {
		t.Errorf("Write after Close: wrote %d bytes, want 0", n)
	}
	assertClosed("Write", err)
	assertClosed("Drain", port.Drain())
	assertClosed("ResetInputBuffer", port.ResetInputBuffer())
	assertClosed("ResetOutputBuffer", port.ResetOutputBuffer())
	assertClosed("SetMode", port.SetMode(&Mode{BaudRate: 9600}))
	assertClosed("SetDTR", port.SetDTR(true))
	assertClosed("SetRTS", port.SetRTS(false))
	assertClosed("Break", port.Break(time.Millisecond))
	_, err = port.GetModemStatusBits()
	assertClosed("GetModemStatusBits", err)
}
