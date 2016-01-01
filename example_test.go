//
// Copyright 2015 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial_test

import "fmt"
import "log"
import "go.bug.st/serial"

func ExampleGetPortsList() {
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		fmt.Println("No serial ports found!")
	} else {
		for _, port := range ports {
			fmt.Printf("Found port: %v\n", port)
		}
	}
}

func ExampleSetMode() {
	port, err := serial.OpenPort("/dev/ttyACM0", &serial.Mode{})
	if err != nil {
		log.Fatal(err)
	}
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.PARITY_NONE,
		DataBits: 8,
		StopBits: serial.STOPBITS_ONE,
	}
	if err := port.SetMode(mode); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Port set to 9600 N81")
}

func ExampleFullCommunication() {
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		log.Fatal("No serial ports found!")
	}

	for _, port := range ports {
		fmt.Printf("Found port: %v\n", port)
	}

	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.PARITY_NONE,
		DataBits: 8,
		StopBits: serial.STOPBITS_ONE,
	}
	port, err := serial.OpenPort(ports[0], mode)
	if err != nil {
		log.Fatal(err)
	}
	n, err := port.Write([]byte("10,20,30\n\r"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Sent %v bytes\n", n)

	buff := make([]byte, 100)
	for {
		// Reads up to 100 bytes
		n, err := port.Read(buff)
		if err != nil {
			log.Fatal(err)
			break
		}
		if n == 0 {
			fmt.Println("\nEOF")
			break
		}
		fmt.Printf("%v", string(buff[:n]))
	}
}

// vi:ts=2
