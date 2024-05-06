//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial_test

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.bug.st/serial"
)

// This example prints the list of serial ports and use the first one
// to send a string "10,20,30" and prints the response on the screen.
func Example_sendAndReceive() {

	// Retrieve the port list
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		log.Fatal("No serial ports found!")
	}

	// Print the list of detected ports
	for _, port := range ports {
		fmt.Printf("Found port: %v\n", port)
	}

	// Open the first serial port detected at 9600bps N81
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	port, err := serial.Open(ports[0], mode)
	if err != nil {
		log.Fatal(err)
	}

	// Send the string "10,20,30\n\r" to the serial port
	n, err := port.Write([]byte("10,20,30\n\r"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Sent %v bytes\n", n)

	// Read and print the response

	if err := port.SetReadTimeout(1 * time.Minute); err != nil {
		fmt.Printf("failed to set read timeout: %v\n", err)
	}

	buff := make([]byte, 100)
	for {
		// Reads up to 100 bytes
		n, err := port.Read(buff)
		if err != nil {
			if os.IsTimeout(err) {
				fmt.Println("timeout")
				// TODO do something on read timeout
				continue
			}
			log.Fatal(err)
		}
		if n == 0 {
			fmt.Println("\nEOF")
			break
		}

		fmt.Printf("%s", string(buff[:n]))

		// If we receive a newline stop reading
		if strings.Contains(string(buff[:n]), "\n") {
			break
		}
	}
}
