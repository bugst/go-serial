package main

import "github.com/bugst/go-serial/serial"
import "fmt"
import "log"

func main() {
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Fatal(err)
	}
	for _, port := range ports {
		fmt.Printf("Found port: %v\n", port)
	}

	port, err := serial.OpenPort("/dev/ttyACM0", false)
	if err != nil {
		log.Fatal(err)
	}
	err = port.SetSpeed(115200)
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
