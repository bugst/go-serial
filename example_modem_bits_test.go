//
// Copyright 2014-2017 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial_test

import "log"
import "go.bug.st/serial.v1"
import "fmt"
import "time"

func ExampleGetSetModemBits() {
	// Open the first serial port detected at 9600bps N81
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	port, err := serial.Open("/dev/ttyACM1", mode)
	if err != nil {
		log.Fatal(err)
	}
	defer port.Close()

	count := 0
	for count < 25 {
		status, err := port.GetModemStatusBits()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Status: %+v\n", status)

		time.Sleep(time.Second)
		count++
		if count == 5 {
			err := port.SetDTR(false)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Set DTR OFF")
		}
		if count == 10 {
			err := port.SetDTR(true)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Set DTR ON")
		}
		if count == 15 {
			err := port.SetRTS(false)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Set RTS OFF")
		}
		if count == 20 {
			err := port.SetRTS(true)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Set RTS ON")
		}
	}
}
