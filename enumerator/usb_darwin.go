//
// Copyright 2014-2023 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

// #cgo LDFLAGS: -framework CoreFoundation -framework IOKit
// #include <IOKit/IOKitLib.h>
// #include <CoreFoundation/CoreFoundation.h>
// #include <stdlib.h>
import "C"
import (
	"errors"
	"fmt"
	"time"
	"unsafe"
)

func nativeGetDetailedPortsList() ([]*PortDetails, error) {
	var ports []*PortDetails

	services, err := getAllServices("IOSerialBSDClient")
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}
	for _, service := range services {
		defer service.Release()

		port, err := extractPortInfo(&io_registry_entry_t{ioregistryentry: service.ioobject})
		if err != nil {
			return nil, &PortEnumerationError{causedBy: err}
		}
		ports = append(ports, port)
	}
	return ports, nil
}

func extractPortInfo(service *io_registry_entry_t) (*PortDetails, error) {
	port := &PortDetails{}
	// If called too early the port may still not be ready or fully enumerated
	// so we retry 5 times before returning error.
	for retries := 5; retries > 0; retries-- {
		name, err := service.GetStringProperty("IOCalloutDevice")
		if err == nil {
			port.Name = name
			break
		}
		if retries == 0 {
			return nil, fmt.Errorf("error extracting port info from device: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	port.IsUSB = false

	validUSBDeviceClass := map[string]bool{
		"IOUSBDevice":     true,
		"IOUSBHostDevice": true,
	}

	var (
		usbDevice    = io_registry_entry_t{service.ioregistryentry}
		usbDeviceObj = io_object_t{usbDevice.ioregistryentry}
		searchErr    error
	)

	for !validUSBDeviceClass[usbDeviceObj.GetClass()] {
		if usbDevice, searchErr = usbDevice.GetParent("IOService"); searchErr != nil {
			break
		}
		usbDeviceObj = io_object_t{usbDevice.ioregistryentry}
	}
	if searchErr == nil {
		// It's an IOUSBDevice
		vid, _ := usbDevice.GetIntProperty("idVendor", C.kCFNumberSInt16Type)
		pid, _ := usbDevice.GetIntProperty("idProduct", C.kCFNumberSInt16Type)
		serialNumber, _ := usbDevice.GetStringProperty("USB Serial Number")
		//product, _ := usbDevice.GetStringProperty("USB Product Name")
		//manufacturer, _ := usbDevice.GetStringProperty("USB Vendor Name")
		//fmt.Println(product + " - " + manufacturer)

		port.IsUSB = true
		port.VID = fmt.Sprintf("%04X", vid)
		port.PID = fmt.Sprintf("%04X", pid)
		port.SerialNumber = serialNumber
	}
	return port, nil
}

func getAllServices(serviceType string) ([]io_object_t, error) {
	i, err := getMatchingServices(serviceMatching(serviceType))
	if err != nil {
		return nil, err
	}
	defer i.Release()

	var services []io_object_t
	tries := 0
	for tries < 5 {
		// Extract all elements from iterator
		if service, ok := i.Next(); ok {
			services = append(services, *service)
			continue
		}
		// If iterator is still valid return the result
		if i.IsValid() {
			return services, nil
		}
		// Otherwise empty the result and retry
		for _, s := range services {
			s.Release()
		}
		services = []io_object_t{}
		i.Reset()
		tries++
	}
	// Give up if the iteration continues to fail...
	return nil, fmt.Errorf("IOServiceGetMatchingServices failed, data changed while iterating")
}

// serviceMatching create a matching dictionary that specifies an IOService class match.
func serviceMatching(serviceType string) C.CFMutableDictionaryRef {
	t := C.CString(serviceType)
	defer C.free(unsafe.Pointer(t))
	return C.IOServiceMatching(t)
}

// getMatchingServices look up registered IOService objects that match a matching dictionary.
func getMatchingServices(matcher C.CFMutableDictionaryRef) (io_iterator_t, error) {
	var i C.io_iterator_t
	err := C.IOServiceGetMatchingServices(C.kIOMasterPortDefault, C.CFDictionaryRef(matcher), &i)
	if err != C.KERN_SUCCESS {
		return io_iterator_t{}, fmt.Errorf("IOServiceGetMatchingServices failed (code %d)", err)
	}
	return io_iterator_t{i}, nil
}

// cfStringRef

type cfStringRef struct {
	cfs C.CFStringRef
}

func CFStringCreateWithString(s string) cfStringRef {
	c := C.CString(s)
	defer C.free(unsafe.Pointer(c))
	val := C.CFStringCreateWithCString(
		C.kCFAllocatorDefault, c, C.kCFStringEncodingMacRoman)
	return cfStringRef{val}
}

func (ref cfStringRef) Release() {
	C.CFRelease(C.CFTypeRef(ref.cfs))
}

// CFTypeRef

type cfTypeRef struct {
	cft C.CFTypeRef
}

func (ref cfTypeRef) Release() {
	C.CFRelease(C.CFTypeRef(ref.cft))
}

// io_registry_entry_t

type io_registry_entry_t struct {
	ioregistryentry C.io_registry_entry_t
}

func (me *io_registry_entry_t) GetParent(plane string) (io_registry_entry_t, error) {
	cPlane := C.CString(plane)
	defer C.free(unsafe.Pointer(cPlane))
	var parent C.io_registry_entry_t
	err := C.IORegistryEntryGetParentEntry(me.ioregistryentry, cPlane, &parent)
	if err != 0 {
		return io_registry_entry_t{}, errors.New("No parent device available")
	}
	return io_registry_entry_t{parent}, nil
}

func (me *io_registry_entry_t) CreateCFProperty(key string) (cfTypeRef, error) {
	k := CFStringCreateWithString(key)
	defer k.Release()
	property := C.IORegistryEntryCreateCFProperty(me.ioregistryentry, k.cfs, C.kCFAllocatorDefault, 0)
	if property == 0 {
		return cfTypeRef{}, errors.New("Property not found: " + key)
	}
	return cfTypeRef{property}, nil
}

func (me *io_registry_entry_t) GetStringProperty(key string) (string, error) {
	property, err := me.CreateCFProperty(key)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	defer property.Release()

	if ptr := C.CFStringGetCStringPtr(C.CFStringRef(property.cft), 0); ptr != nil {
		return C.GoString(ptr), nil
	}
	// in certain circumstances CFStringGetCStringPtr may return NULL
	// and we must retrieve the string by copy
	buff := make([]C.char, 1024)
	if C.CFStringGetCString(C.CFStringRef(property.cft), &buff[0], 1024, 0) != C.true {
		return "", fmt.Errorf("Property '%s' can't be converted", key)
	}
	return C.GoString(&buff[0]), nil
}

func (me *io_registry_entry_t) GetIntProperty(key string, intType C.CFNumberType) (int, error) {
	property, err := me.CreateCFProperty(key)
	if err != nil {
		return 0, err
	}
	defer property.Release()
	var res int
	if C.CFNumberGetValue((C.CFNumberRef)(property.cft), intType, unsafe.Pointer(&res)) != C.true {
		return res, fmt.Errorf("Property '%s' can't be converted or has been truncated", key)
	}
	return res, nil
}

// io_iterator_t

type io_iterator_t struct {
	ioiterator C.io_iterator_t
}

// IsValid checks if an iterator is still valid.
// Some iterators will be made invalid if changes are made to the
// structure they are iterating over. This function checks the iterator
// is still valid and should be called when Next returns zero.
// An invalid iterator can be Reset and the iteration restarted.
func (me *io_iterator_t) IsValid() bool {
	return C.IOIteratorIsValid(me.ioiterator) == C.true
}

func (me *io_iterator_t) Reset() {
	C.IOIteratorReset(me.ioiterator)
}

func (me *io_iterator_t) Next() (*io_object_t, bool) {
	res := C.IOIteratorNext(me.ioiterator)
	return &io_object_t{res}, res != 0
}

func (me *io_iterator_t) Release() {
	C.IOObjectRelease(me.ioiterator)
}

// io_object_t

type io_object_t struct {
	ioobject C.io_object_t
}

func (me *io_object_t) Release() {
	C.IOObjectRelease(me.ioobject)
}

func (me *io_object_t) GetClass() string {
	class := make([]C.char, 1024)
	C.IOObjectGetClass(me.ioobject, &class[0])
	return C.GoString(&class[0])
}
