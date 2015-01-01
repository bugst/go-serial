//
// Copyright 2014 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package test

import "go.bug.st/serial"

func BuildTest() {
	serial.GetPortsList()
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.PARITY_NONE,
		DataBits: 8,
		StopBits: serial.STOPBITS_ONE,
	}
	port, _ := serial.OpenPort("", mode)
	port.SetMode(mode)
	buff := make([]byte, 100)
	port.Write(buff)
	port.Read(buff)
	port.Close()
}

// vi:ts=2
