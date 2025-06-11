//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

//go:build darwin && !cgo

package enumerator

import (
	"bytes"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

var libraryLoadError = lib.Load()

func nativeGetDetailedPortsList() ([]*PortDetails, error) {
	if libraryLoadError != nil {
		return nil, libraryLoadError
	}

	var ports []*PortDetails

	services, err := getAllServices("IOSerialBSDClient")
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}
	for _, service := range services {
		port, err := extractPortInfo(service)
		service.Release()
		if err != nil {
			return nil, &PortEnumerationError{causedBy: err}
		}
		ports = append(ports, port)
	}
	return ports, nil
}

func extractPortInfo(service io_registry_entry_t) (*PortDetails, error) {
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
	usbDevice := service
	var searchErr error
	for !validUSBDeviceClass[usbDevice.GetClass()] {
		if usbDevice, searchErr = usbDevice.GetParent("IOService"); searchErr != nil {
			break
		}
	}
	if searchErr == nil {
		// It's an IOUSBDevice
		vid, _ := usbDevice.GetIntProperty("idVendor", kCFNumberSInt16Type)
		pid, _ := usbDevice.GetIntProperty("idProduct", kCFNumberSInt16Type)
		serialNumber, _ := usbDevice.GetStringProperty("USB Serial Number")

		product := usbDevice.GetName()
		manufacturer, _ := usbDevice.GetStringProperty("USB Vendor Name")

		port.IsUSB = true
		port.VID = fmt.Sprintf("%04X", vid)
		port.PID = fmt.Sprintf("%04X", pid)
		port.SerialNumber = serialNumber
		port.Product = product
		port.Manufacturer = manufacturer
	}
	return port, nil
}

func getAllServices(serviceType string) ([]io_registry_entry_t, error) {
	i, err := getMatchingServices(lib.IOServiceMatching(serviceType))
	if err != nil {
		return nil, err
	}
	defer i.Release()

	var services []io_registry_entry_t
	tries := 0
	for tries < 5 {
		// Extract all elements from iterator
		if service, ok := i.Next(); ok {
			services = append(services, io_registry_entry_t(service))
			continue
		}
		// If the list of services is empty or the iterator is still valid return the result
		if len(services) == 0 || i.IsValid() {
			return services, nil
		}
		// Otherwise empty the result and retry
		for _, s := range services {
			s.Release()
		}
		services = services[:0]
		i.Reset()
		tries++
	}
	// Give up if the iteration continues to fail...
	return nil, fmt.Errorf("IOServiceGetMatchingServices failed, data changed while iterating")
}

// getMatchingServices look up registered IOService objects that match a matching dictionary.
func getMatchingServices(matcher cfMutableDictionaryRef) (io_iterator_t, error) {
	var i io_iterator_t
	res := lib.IOServiceGetMatchingServices(lib.kIOMasterPortDefault, cfDictionaryRef(matcher), &i)
	if res.Failed() {
		return 0, fmt.Errorf("IOServiceGetMatchingServices failed (code %d)", res)
	}
	return i, nil
}

type library struct {
	// IOKit
	kIOMasterPortDefault uintptr

	IOIteratorIsValid               func(io_iterator_t) bool
	IOIteratorNext                  func(io_iterator_t) io_object_t
	IOIteratorReset                 func(io_iterator_t)
	IOObjectGetClass                func(io_object_t, *io_name_t) kern_return_t
	IOObjectRelease                 func(io_object_t) int
	IORegistryEntryCreateCFProperty func(io_registry_entry_t, cfStringRef, cfAllocatorRef, uint32) cfTypeRef
	IORegistryEntryGetName          func(io_registry_entry_t, *io_name_t) kern_return_t
	IORegistryEntryGetParentEntry   func(io_registry_entry_t, string, *io_registry_entry_t) kern_return_t
	IOServiceGetMatchingServices    func(uintptr, cfDictionaryRef, *io_iterator_t) kern_return_t
	IOServiceMatching               func(string) cfMutableDictionaryRef

	// CoreFoundation
	kCFAllocatorDefault cfAllocatorRef

	CFNumberGetValue          func(cfNumberRef, cfNumberType, unsafe.Pointer) bool
	CFRelease                 func(cfTypeRef)
	CFStringCreateWithCString func(cfAllocatorRef, string, cfStringEncoding) cfStringRef
	CFStringGetCString        func(cfStringRef, *byte, int, cfStringEncoding) bool
	CFStringGetCStringPtr     func(cfStringRef, cfStringEncoding) string
}

var lib library

func (l *library) Load() error {
	if err := l.loadIOKit(); err != nil {
		return err
	}
	if err := l.loadCF(); err != nil {
		return err
	}
	return nil
}

func (l *library) loadIOKit() error {
	iokitLib, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}

	l.kIOMasterPortDefault = 0 // purego.Dlsym(iokitLib, "kIOMasterPortDefault")

	purego.RegisterLibFunc(&l.IOIteratorIsValid, iokitLib, "IOIteratorIsValid")
	purego.RegisterLibFunc(&l.IOIteratorNext, iokitLib, "IOIteratorNext")
	purego.RegisterLibFunc(&l.IOObjectGetClass, iokitLib, "IOObjectGetClass")
	purego.RegisterLibFunc(&l.IOObjectRelease, iokitLib, "IOObjectRelease")
	purego.RegisterLibFunc(&l.IORegistryEntryCreateCFProperty, iokitLib, "IORegistryEntryCreateCFProperty")
	purego.RegisterLibFunc(&l.IORegistryEntryGetName, iokitLib, "IORegistryEntryGetName")
	purego.RegisterLibFunc(&l.IORegistryEntryGetParentEntry, iokitLib, "IORegistryEntryGetParentEntry")
	purego.RegisterLibFunc(&l.IOServiceGetMatchingServices, iokitLib, "IOServiceGetMatchingServices")
	purego.RegisterLibFunc(&l.IOServiceMatching, iokitLib, "IOServiceMatching")
	return nil
}

func (l *library) loadCF() error {
	cfLib, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}

	ptr, err := purego.Dlsym(cfLib, "kCFAllocatorDefault")
	if err != nil {
		return err
	}
	l.kCFAllocatorDefault = *((*cfAllocatorRef)(unsafe.Pointer(ptr)))

	purego.RegisterLibFunc(&l.CFNumberGetValue, cfLib, "CFNumberGetValue")
	purego.RegisterLibFunc(&l.CFRelease, cfLib, "CFRelease")
	purego.RegisterLibFunc(&l.CFStringCreateWithCString, cfLib, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&l.CFStringGetCString, cfLib, "CFStringGetCString")
	purego.RegisterLibFunc(&l.CFStringGetCStringPtr, cfLib, "CFStringGetCStringPtr")
	return nil
}

func (l *library) GoString(buf []byte) string {
	i := bytes.IndexByte(buf, 0)
	if i < 0 {
		return string(buf)
	}
	return string(buf[:i])
}

func (l *library) ToCFString(s string) cfStringRef {
	return l.CFStringCreateWithCString(l.kCFAllocatorDefault, s, kCFStringEncodingUTF8)
}

func (l *library) FromCFString(p cfStringRef) string {
	return l.CFStringGetCStringPtr(p, kCFStringEncodingUTF8)
}

func (l *library) CFStringToBuf(p cfStringRef, buf []byte) bool {
	return l.CFStringGetCString(p, &buf[0], len(buf), kCFStringEncodingUTF8)
}

type (
	kern_return_t       uint32
	io_name_t           [128]byte
	io_object_t         uintptr
	io_iterator_t       io_object_t
	io_registry_entry_t io_object_t
)

const (
	KERN_SUCCESS kern_return_t = 0
)

func (r kern_return_t) Successed() bool {
	return r == KERN_SUCCESS
}

func (r kern_return_t) Failed() bool {
	return r != KERN_SUCCESS
}

func (s *io_name_t) AsPtr() *byte {
	return &s[0]
}

func (s *io_name_t) String() string {
	return lib.GoString(s[:])
}

func (o io_object_t) Release() {
	lib.IOObjectRelease(o)
}

func (o io_object_t) GetClass() string {
	var class io_name_t
	if lib.IOObjectGetClass(o, &class).Failed() {
		return ""
	}
	return class.String()
}

// IsValid checks if an iterator is still valid.
// Some iterators will be made invalid if changes are made to the
// structure they are iterating over. This function checks the iterator
// is still valid and should be called when Next returns zero.
// An invalid iterator can be Reset and the iteration restarted.
func (i io_iterator_t) IsValid() bool {
	return lib.IOIteratorIsValid(i)
}

func (i io_iterator_t) Next() (io_object_t, bool) {
	o := lib.IOIteratorNext(i)
	if o == 0 {
		return 0, false
	}
	return o, true
}

func (i io_iterator_t) Reset() {
	lib.IOIteratorReset(i)
}

func (i io_iterator_t) Release() {
	io_object_t(i).Release()
}

func (e io_registry_entry_t) GetParent(plane string) (io_registry_entry_t, error) {
	var parent io_registry_entry_t
	if lib.IORegistryEntryGetParentEntry(e, plane, &parent).Failed() {
		return 0, errors.New("No parent device available")
	}
	return parent, nil
}

func (e io_registry_entry_t) CreateCFProperty(key string) (cfTypeRef, error) {
	k := lib.ToCFString(key)
	defer k.Release()
	property := lib.IORegistryEntryCreateCFProperty(e, k, lib.kCFAllocatorDefault, 0)
	if property == 0 {
		return 0, errors.New("Property not found: " + key)
	}
	return property, nil
}

func (e io_registry_entry_t) GetStringProperty(key string) (string, error) {
	property, err := e.CreateCFProperty(key)
	if err != nil {
		return "", err
	}
	defer property.Release()

	if str := lib.CFStringGetCStringPtr(cfStringRef(property), kCFStringEncodingMacRoman); str == "" {
		return str, nil
	}
	var name io_name_t
	if !lib.CFStringGetCString(cfStringRef(property), name.AsPtr(), len(name), kCFStringEncodingUTF8) {
		return "", fmt.Errorf("Property '%s' can't be converted", key)
	}
	return name.String(), nil
}

func (e io_registry_entry_t) GetIntProperty(key string, intType cfNumberType) (int64, error) {
	property, err := e.CreateCFProperty(key)
	if err != nil {
		return 0, err
	}
	defer property.Release()
	var res int64
	if !lib.CFNumberGetValue(cfNumberRef(property), intType, unsafe.Pointer(&res)) {
		return res, fmt.Errorf("Property '%s' can't be converted or has been truncated", key)
	}
	return res, nil
}

func (e io_registry_entry_t) Release() {
	io_object_t(e).Release()
}

func (e io_registry_entry_t) GetClass() string {
	return io_object_t(e).GetClass()
}

func (e io_registry_entry_t) GetName() string {
	var name io_name_t
	if lib.IORegistryEntryGetName(e, &name).Failed() {
		return ""
	}
	return name.String()
}

type (
	cfIndex          int
	cfStringEncoding uint32
	cfTypeRef        uintptr

	cfAllocatorRef         cfTypeRef
	cfDictionaryRef        cfTypeRef
	cfMutableDictionaryRef cfTypeRef
	cfNumberRef            cfTypeRef
	cfNumberType           cfIndex
	cfStringRef            cfTypeRef
)

const (
	kCFNumberSInt8Type  cfNumberType = 1
	kCFNumberSInt16Type cfNumberType = 2
	kCFNumberSInt32Type cfNumberType = 3
	kCFNumberSInt64Type cfNumberType = 4
)

const (
	kCFStringEncodingMacRoman cfStringEncoding = 0x0
	kCFStringEncodingUTF8     cfStringEncoding = 0x08000100
)

func (t cfTypeRef) Release() {
	lib.CFRelease(t)
}

func (s cfStringRef) String() string {
	return lib.CFStringGetCStringPtr(s, kCFStringEncodingUTF8)
}

func (s cfStringRef) Release() {
	cfTypeRef(s).Release()
}
