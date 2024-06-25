//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial_test

import (
	"fmt"
	"log"

	"go.bug.st/serial"
)

func ExamplePort_SetMode() {
	port, err := serial.Open("/dev/ttyACM0", &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Port set to 9600 N81")
}
