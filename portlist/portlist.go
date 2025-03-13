//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

// portlist is a tool to list all the available serial ports.
// Just run it and it will produce an output like:
//
// $ go run portlist.go
// Port: /dev/cu.Bluetooth-Incoming-Port
// Port: /dev/cu.usbmodemFD121
//    USB ID     2341:8053
//    USB serial FB7B6060504B5952302E314AFF08191A

package main

import (
	"fmt"
	"log"

	"github.com/abakum/go-serial"
	"github.com/abakum/go-serial/enumerator"
)

func main() {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		return
	}
	PortsList, err := serial.GetPortsList()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Port list:", PortsList)
	for _, port := range ports {
		fmt.Printf("Port: %s\n", serial.PortName(port.Name))
		fmt.Printf("\tPath: %s\n", serial.DevName(port.Name))
		if port.Product != "" {
			fmt.Printf("\tProduct Name: %s\n", port.Product)
		}
		if port.IsUSB {
			fmt.Printf("\tUSB ID: %s:%s\n", port.VID, port.PID)
			if port.SerialNumber != "" {
				fmt.Printf("\tUSB serial: %s\n", port.SerialNumber)
			}
		}
		mode := serial.Mode{BaudRate: -1}
		sp, err := serial.Open(port.Name, &mode)
		if err != nil {
			fmt.Printf("\t%s\n", err)
			continue
		}
		fmt.Printf("\tMode: %+v\n", mode)
		sp.Close()
	}
	fmt.Printf("First serial port is %q\n", serial.PortName(""))
}
