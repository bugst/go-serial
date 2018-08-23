//
// Copyright 2014-2018 Cristian Maglie. All rights reserved.
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
//
package main

import "fmt"
import "log"
import "go.bug.st/serial.v1/enumerator"

func main() {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		return
	}
	for _, port := range ports {
		fmt.Printf("Port: %s\n", port.Name)
		if port.IsUSB {
			fmt.Printf("   USB ID     %s:%s\n", port.VID, port.PID)
			fmt.Printf("   USB serial %s\n", port.SerialNumber)
		}
	}
}
