//
// Copyright 2014-2026 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

// #cgo LDFLAGS: -framework CoreFoundation -framework IOKit
// #include <IOKit/IOKitLib.h>
// #include <IOKit/IOCFPlugIn.h>
// #include <IOKit/usb/IOUSBLib.h>
// #include <CoreFoundation/CoreFoundation.h>
// #include <stdlib.h>
//
// HRESULT callIOCFPlugin_QueryInterface(IOCFPlugInInterface **plugin, REFIID iid, LPVOID *ppv) {
//   return (*plugin)->QueryInterface(plugin, iid, ppv);
// }
// HRESULT callIOCFPlugin_Release(IOCFPlugInInterface **plugin) {
//   return (*plugin)->Release(plugin);
// }
//
// IOReturn callIOUSBDevice_USBDeviceOpen(IOUSBDeviceInterface **device) {
//   return (*device)->USBDeviceOpen(device);
// }
// IOReturn callIOUSBDevice_USBDeviceClose(IOUSBDeviceInterface **device) {
//   return (*device)->USBDeviceClose(device);
// }
// ULONG callIOUSBDdevice_Release(IOUSBDeviceInterface **device) {
//   return (*device)->Release(device);
// }
// IOReturn callIOUSBDevice_GetConfiguration(IOUSBDeviceInterface **device, UInt8 *config) {
//   return (*device)->GetConfiguration(device, config);
// }
// IOReturn callIOUSBDevice_GetNumberOfConfigurations(IOUSBDeviceInterface **device, UInt8 *numConfigs) {
//   return (*device)->GetNumberOfConfigurations(device, numConfigs);
// }
// IOReturn callIOUSBDevice_GetConfigurationDescriptorPtr(IOUSBDeviceInterface **device, UInt8 index, IOUSBConfigurationDescriptorPtr *configDesc) {
//   return (*device)->GetConfigurationDescriptorPtr(device, index, configDesc);
// }
// IOReturn callIOUSBDevice_DeviceRequest(IOUSBDeviceInterface **device, IOUSBDevRequest *request) {
//   return (*device)->DeviceRequest(device, request);
// }
import "C"
import (
	"errors"
	"fmt"
	"time"
	"unsafe"
)

func nativeGetDetailedPortsList(shouldProbeUSB func(vid, pid string) bool) ([]*PortDetails, error) {
	var ports []*PortDetails

	services, err := getAllServices("IOSerialBSDClient")
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}
	defer func() {
		for _, service := range services {
			service.Release()
		}
	}()

	for _, service := range services {
		port, err := extractPortInfo(io_registry_entry_t(service), shouldProbeUSB)
		if err != nil {
			return nil, &PortEnumerationError{causedBy: err}
		}
		ports = append(ports, port)
	}
	return ports, nil
}

func extractPortInfo(service io_registry_entry_t, shouldProbeUSB func(vid, pid string) bool) (*PortDetails, error) {
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
		parent, err := usbDevice.GetParent("IOService")
		if err != nil {
			searchErr = err
			break
		}
		if usbDevice != service {
			usbDevice.Release()
		}
		usbDevice = parent
	}
	if usbDevice != service {
		defer usbDevice.Release()
	}
	if searchErr == nil {
		// It's an IOUSBDevice
		vid, _ := usbDevice.GetIntProperty("idVendor", C.kCFNumberSInt16Type)
		pid, _ := usbDevice.GetIntProperty("idProduct", C.kCFNumberSInt16Type)
		serialNumber, _ := usbDevice.GetStringProperty("kUSBSerialNumberString")
		vendor, _ := usbDevice.GetStringProperty("kUSBVendorString")
		product, _ := usbDevice.GetStringProperty("kUSBProductString")

		port.IsUSB = true
		port.VID = fmt.Sprintf("%04X", vid)
		port.PID = fmt.Sprintf("%04X", pid)
		port.SerialNumber = serialNumber
		port.Manufacturer = vendor
		port.Product = product

		// Retrieving the USB configuration string requires actively opening
		// the device and issuing control requests, which may interfere with
		// the device's normal operation. Only do this if the caller
		// explicitly allowed probing for this VID/PID.
		if shouldProbeUSB(port.VID, port.PID) {
			port.Configuration, _ = usbDevice.GetUSBConfigurationString()
		}
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
			services = append(services, service)
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
		return 0, fmt.Errorf("IOServiceGetMatchingServices failed (code %d)", err)
	}
	return io_iterator_t(i), nil
}

// CFStringRef

type cfStringRef C.CFStringRef

func cfStringCreateWithString(s string) cfStringRef {
	c := C.CString(s)
	defer C.free(unsafe.Pointer(c))
	return cfStringRef(C.CFStringCreateWithCString(C.kCFAllocatorDefault, c, C.kCFStringEncodingMacRoman))
}

func cfStringCreateWithBytes(data unsafe.Pointer, len uint32, encoding C.CFStringEncoding) (cfStringRef, bool) {
	str := C.CFStringCreateWithBytes(C.kCFAllocatorDefault, (*C.uint8_t)(data), C.CFIndex(len), encoding, C.FALSE)
	return cfStringRef(str), str != 0
}

func (ref cfStringRef) GetLength() uint32 {
	return uint32(C.CFStringGetLength(C.CFStringRef(ref)))
}

func (ref cfStringRef) GetMaximumSizeForEncoding(encoding C.CFStringEncoding) uint32 {
	return uint32(C.CFStringGetMaximumSizeForEncoding(C.CFIndex(ref.GetLength()), encoding))
}

func (ref cfStringRef) GetGoString() (string, bool) {
	maxSize := ref.GetMaximumSizeForEncoding(C.kCFStringEncodingUTF8) + 1
	buff := C.malloc(C.size_t(maxSize))
	if buff == nil {
		return "", false
	}
	defer C.free(buff)
	if C.CFStringGetCString(C.CFStringRef(ref), (*C.char)(buff), C.CFIndex(maxSize), C.kCFStringEncodingUTF8) == C.false {
		return "", false
	}
	return C.GoString((*C.char)(buff)), true
}

func (ref cfStringRef) Release() {
	C.CFRelease(C.CFTypeRef(ref))
}

// CFTypeRef

type cfTypeRef C.CFTypeRef

func (ref cfTypeRef) Release() {
	C.CFRelease(C.CFTypeRef(ref))
}

// io_registry_entry_t

type io_registry_entry_t C.io_registry_entry_t

func (me *io_registry_entry_t) GetParent(plane string) (io_registry_entry_t, error) {
	cPlane := C.CString(plane)
	defer C.free(unsafe.Pointer(cPlane))
	var parent C.io_registry_entry_t
	err := C.IORegistryEntryGetParentEntry(C.io_registry_entry_t(*me), cPlane, &parent)
	if err != 0 {
		return 0, errors.New("no parent device available")
	}
	return io_registry_entry_t(parent), nil
}

func (me *io_registry_entry_t) CreateCFProperty(key string) (cfTypeRef, error) {
	k := cfStringCreateWithString(key)
	defer k.Release()
	property := C.IORegistryEntryCreateCFProperty(C.io_registry_entry_t(*me), C.CFStringRef(k), C.kCFAllocatorDefault, 0)
	if property == 0 {
		return 0, errors.New("Property not found: " + key)
	}
	return cfTypeRef(property), nil
}

func (me *io_registry_entry_t) GetStringProperty(key string) (string, error) {
	property, err := me.CreateCFProperty(key)
	if err != nil {
		return "", err
	}
	defer property.Release()

	if ptr := C.CFStringGetCStringPtr(C.CFStringRef(property), 0); ptr != nil {
		return C.GoString(ptr), nil
	}
	// in certain circumstances CFStringGetCStringPtr may return NULL
	// and we must retrieve the string by copy
	buff := make([]C.char, 1024)
	if C.CFStringGetCString(C.CFStringRef(property), &buff[0], 1024, 0) != C.true {
		return "", fmt.Errorf("property '%s' can't be converted", key)
	}
	return C.GoString(&buff[0]), nil
}

func (me *io_registry_entry_t) GetUSBConfigurationString() (string, error) {
	configuration, err := RetrieveUSBConfigurationString(io_service_t(*me))
	if err != nil {
		return "", fmt.Errorf("USB configuration string not available: %w", err)
	}
	return configuration, nil
}

func (me *io_registry_entry_t) GetIntProperty(key string, intType C.CFNumberType) (int, error) {
	property, err := me.CreateCFProperty(key)
	if err != nil {
		return 0, err
	}
	defer property.Release()
	var res int
	if C.CFNumberGetValue((C.CFNumberRef)(property), intType, unsafe.Pointer(&res)) != C.true {
		return res, fmt.Errorf("property '%s' can't be converted or has been truncated", key)
	}
	return res, nil
}

func (me *io_registry_entry_t) Release() {
	C.IOObjectRelease(C.io_object_t(*me))
}

func (me *io_registry_entry_t) GetClass() string {
	class := make([]C.char, 1024)
	C.IOObjectGetClass(C.io_object_t(*me), &class[0])
	return C.GoString(&class[0])
}

// io_iterator_t

type io_iterator_t C.io_iterator_t

// IsValid checks if an iterator is still valid.
// Some iterators will be made invalid if changes are made to the
// structure they are iterating over. This function checks the iterator
// is still valid and should be called when Next returns zero.
// An invalid iterator can be Reset and the iteration restarted.
func (me *io_iterator_t) IsValid() bool {
	return C.IOIteratorIsValid(C.io_iterator_t(*me)) == C.true
}

func (me *io_iterator_t) Reset() {
	C.IOIteratorReset(C.io_iterator_t(*me))
}

func (me *io_iterator_t) Next() (io_object_t, bool) {
	res := C.IOIteratorNext(C.io_iterator_t(*me))
	return io_object_t(res), res != 0
}

func (me *io_iterator_t) Release() {
	C.IOObjectRelease(C.io_object_t(*me))
}

// io_object_t

type io_object_t C.io_object_t

func (me *io_object_t) Release() {
	C.IOObjectRelease(C.io_object_t(*me))
}

func (me *io_object_t) GetClass() string {
	class := make([]C.char, 1024)
	C.IOObjectGetClass(C.io_object_t(*me), &class[0])
	return C.GoString(&class[0])
}

// io_service_t

type io_service_t C.io_service_t

func (me *io_service_t) IOCreatePlugInInterfaceForService() (plugin *IOCFPlugIn, score int32, err error) {
	res := IOCFPlugIn{}
	var s C.SInt32
	kr := C.IOCreatePlugInInterfaceForService(C.io_service_t(*me), C.kIOUSBDeviceUserClientTypeID, C.kIOCFPlugInInterfaceID, &res.h, &s)
	if kr != C.kIOReturnSuccess || res.h == nil {
		return nil, 0, fmt.Errorf("IOCreatePlugInInterfaceForService failed (code %d)", kr)
	}
	return &res, int32(s), nil
}

// IOCFPlugInInterface

type IOCFPlugIn struct {
	h **C.IOCFPlugInInterface
}

func (me *IOCFPlugIn) QueryIOUSBDeviceInterface() (*IOUSBDevice, error) {
	var device **C.IOUSBDeviceInterface
	result := C.callIOCFPlugin_QueryInterface(me.h, C.CFUUIDGetUUIDBytes(C.kIOUSBDeviceInterfaceID), (*C.LPVOID)(unsafe.Pointer(&device)))
	if result != C.S_OK {
		return nil, fmt.Errorf("QueryInterface failed (code %d)", result)
	}
	return &IOUSBDevice{h: device}, nil
}

func (me *IOCFPlugIn) Release() {
	C.callIOCFPlugin_Release(me.h)
}

// IOUSBDeviceInterface

type IOUSBDevice struct {
	h **C.IOUSBDeviceInterface
}

func (me *IOUSBDevice) USBDeviceOpen() error {
	kr := C.callIOUSBDevice_USBDeviceOpen(me.h)
	if kr != C.kIOReturnSuccess {
		return fmt.Errorf("USBDeviceOpen failed (code %d)", kr)
	}
	return nil
}

func (me *IOUSBDevice) USBDeviceClose() error {
	kr := C.callIOUSBDevice_USBDeviceClose(me.h)
	if kr != C.kIOReturnSuccess {
		return fmt.Errorf("USBDeviceClose failed (code %d)", kr)
	}
	return nil
}

func (me *IOUSBDevice) Release() {
	C.callIOUSBDdevice_Release(me.h)
}

func (me *IOUSBDevice) GetConfiguration() (uint8, error) {
	var config C.UInt8
	kr := C.callIOUSBDevice_GetConfiguration(me.h, &config)
	if kr != C.kIOReturnSuccess {
		return 0, fmt.Errorf("GetConfiguration failed (code %d)", kr)
	}
	return uint8(config), nil
}

func (me *IOUSBDevice) GetNumberOfConfigurations() (uint8, error) {
	var numConfigs C.UInt8
	kr := C.callIOUSBDevice_GetNumberOfConfigurations(me.h, &numConfigs)
	if kr != C.kIOReturnSuccess {
		return 0, fmt.Errorf("GetNumberOfConfigurations failed (code %d)", kr)
	}
	return uint8(numConfigs), nil
}

func (me *IOUSBDevice) GetConfigurationDescriptorPtr(index uint8) (C.IOUSBConfigurationDescriptorPtr, error) {
	var configDesc C.IOUSBConfigurationDescriptorPtr
	kr := C.callIOUSBDevice_GetConfigurationDescriptorPtr(me.h, C.UInt8(index), &configDesc)
	if kr != C.kIOReturnSuccess {
		return nil, fmt.Errorf("GetConfigurationDescriptorPtr failed (code %d)", kr)
	}
	return configDesc, nil
}

func (me *IOUSBDevice) DeviceRequest(request *C.IOUSBDevRequest) error {
	kr := C.callIOUSBDevice_DeviceRequest(me.h, request)
	if kr != C.kIOReturnSuccess {
		return fmt.Errorf("DeviceRequest failed (code %d)", kr)
	}
	return nil
}

func getUSBStringDescriptor(device *IOUSBDevice, descriptorIndex uint8, langID uint16, pData unsafe.Pointer, buffer []uint8) (uint32, error) {
	if len(buffer) < 2 {
		return 0, errors.New("buffer too small for USB string descriptor")
	}

	request := C.IOUSBDevRequest{
		bmRequestType: (C.kUSBIn << 7) | (C.kUSBStandard << 5) | C.kUSBDevice,
		bRequest:      C.kUSBRqGetDescriptor,
		wValue:        C.UInt16(C.kUSBStringDesc<<8) | C.UInt16(descriptorIndex),
		wIndex:        C.UInt16(langID),
		wLength:       C.UInt16(len(buffer)),
		pData:         pData,
	}
	if err := device.DeviceRequest(&request); err != nil {
		return 0, err
	}
	if request.wLenDone < 2 {
		return 0, errors.New("invalid response length for USB string descriptor")
	}
	if buffer[1] != uint8(C.kUSBStringDesc) {
		return 0, fmt.Errorf("unexpected descriptor type %d", buffer[1])
	}

	descriptorLength := min(uint32(buffer[0]), uint32(request.wLenDone))
	if descriptorLength < 2 {
		return 0, errors.New("descriptor length too short")
	}
	return descriptorLength, nil
}

func appendUniqueLangID(ids []uint16, id uint16) []uint16 {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func RetrieveUSBConfigurationString(service io_service_t) (string, error) {
	plugin, _, err := service.IOCreatePlugInInterfaceForService()
	if err != nil {
		return "", err
	}
	defer plugin.Release()

	device, err := plugin.QueryIOUSBDeviceInterface()
	if err != nil {
		return "", fmt.Errorf("QueryInterface for IOUSBDeviceInterface failed: %w", err)
	}
	if device == nil {
		return "", errors.New("IOUSBDeviceInterface not found")
	}
	defer device.Release()

	if err := device.USBDeviceOpen(); err != nil {
		return "", fmt.Errorf("USBDeviceOpen failed: %w", err)
	}
	defer device.USBDeviceClose()

	currentConfig, err := device.GetConfiguration()
	if err != nil {
		return "", fmt.Errorf("GetConfiguration failed: %w", err)
	}

	numConfigs, err := device.GetNumberOfConfigurations()
	if err != nil {
		return "", fmt.Errorf("GetNumberOfConfigurations failed: %w", err)
	}

	var stringIndex uint8
	var fallbackIndex uint8
	for index := uint8(0); index < numConfigs; index++ {
		configDesc, err := device.GetConfigurationDescriptorPtr(index)
		if err != nil || configDesc == nil {
			continue
		}

		idx := uint8(configDesc.iConfiguration)
		if idx != 0 && fallbackIndex == 0 {
			fallbackIndex = idx
		}

		// Some devices expose multiple descriptors (e.g. HS/FS variants) with
		// the same bConfigurationValue. Prefer the first non-zero index that
		// matches the active configuration and stop to avoid overwriting it.
		if currentConfig != 0 && uint8(configDesc.bConfigurationValue) == currentConfig {
			if idx != 0 {
				stringIndex = idx
				break
			}
			if stringIndex == 0 {
				stringIndex = idx
			}
		}
	}

	if stringIndex == 0 {
		stringIndex = fallbackIndex
	}

	if stringIndex == 0 {
		return "", errors.New("configuration string index not found")
	}

	const maxUSBStringLength = 255
	pData := C.malloc(maxUSBStringLength)
	if pData == nil {
		return "", errors.New("failed to allocate memory for USB request")
	}
	buffer := unsafe.Slice((*uint8)(pData), maxUSBStringLength)
	defer C.free(pData)

	langIDs := []uint16{0x0409}
	if descriptorLength, err := getUSBStringDescriptor(device, 0, 0, pData, buffer); err == nil && descriptorLength >= 4 {
		for i := uint32(2); i+1 < descriptorLength; i += 2 {
			langID := uint16(buffer[i]) | (uint16(buffer[i+1]) << 8)
			langIDs = appendUniqueLangID(langIDs, langID)
		}
	}

	var lastErr error
	for _, langID := range langIDs {
		descriptorLength, err := getUSBStringDescriptor(device, stringIndex, langID, pData, buffer)
		if err != nil {
			lastErr = err
			continue
		}
		if descriptorLength <= 2 {
			lastErr = errors.New("descriptor length too short for configuration string")
			continue
		}

		cfConfiguration, ok := cfStringCreateWithBytes(unsafe.Add(pData, 2), descriptorLength-2, C.kCFStringEncodingUTF16LE)
		if !ok {
			lastErr = errors.New("failed to create CFString from bytes")
			continue
		}
		defer cfConfiguration.Release()
		configuration, ok := cfConfiguration.GetGoString()
		if !ok {
			lastErr = errors.New("failed to convert CFString to Go string")
			continue
		}
		return configuration, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("configuration string retrieval failed for all LANGIDs: %w", lastErr)
	}
	return "", errors.New("configuration string retrieval failed")
}
