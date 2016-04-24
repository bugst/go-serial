//
// Copyright 2014-2016 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

/*
Package serial is a cross-platform serial library for the go language.

The canonical import for this library is go.bug.st/serial so the import line
is the following:

	import "go.bug.st/serial"

It is possibile to get the list of available serial ports with the
GetPortsList function:

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

The serial port can be opened with the Open function:

	mode := &serial.Mode{
		BaudRate: 115200,
	}
	port, err := serial.Open("/dev/ttyUSB0", mode)
	if err != nil {
		log.Fatal(err)
	}

The Open function needs a "mode" parameter that specifies the configuration
options for the serial port. If not specified the default options are 9600_N81,
in the example above only the speed is changed so the port is opened using 115200_N81.
The following snippets shows how to declare a configuration for 57600_E71:

	mode := &serial.Mode{
		BaudRate: 57600,
		Parity: serial.EvenParity,
		DataBits: 7,
		StopBits: serial.OneStopBit,
	}

The configuration can be changed at any time with the SetMode function:

	err := port.SetMode(mode)
	if err != nil {
		log.Fatal(err)
	}

The port object implements the io.ReadWriteCloser interface, so we can use
the usual Read, Write and Close functions to send and receive data from the
serial port:

	n, err := port.Write([]byte("10,20,30\n\r"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Sent %v bytes\n", n)

	buff := make([]byte, 100)
	for {
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

This library doesn't make use of cgo and "C" package, so it's a pure go library
that can be easily cross compiled.
*/
package serial // import "go.bug.st/serial"
