//
// Copyright 2014-2024 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

import (
	"fmt"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func parseDeviceID(deviceID string, details *PortDetails) {
	// Windows stock USB-CDC driver
	if len(deviceID) >= 3 && deviceID[:3] == "USB" {
		re := regexp.MustCompile(`VID_(....)&PID_(....)(\\(\w+)$)?`).FindAllStringSubmatch(deviceID, -1)
		if re == nil || len(re[0]) < 2 {
			// Silently ignore unparsable strings
			return
		}
		details.IsUSB = true
		details.VID = re[0][1]
		details.PID = re[0][2]
		if len(re[0]) >= 4 {
			details.SerialNumber = re[0][4]
		}
		return
	}

	// FTDI driver
	if len(deviceID) >= 7 && deviceID[:7] == "FTDIBUS" {
		re := regexp.MustCompile(`VID_(....)\+PID_(....)(\+(\w+))?`).FindAllStringSubmatch(deviceID, -1)
		if re == nil || len(re[0]) < 2 {
			// Silently ignore unparsable strings
			return
		}
		details.IsUSB = true
		details.VID = re[0][1]
		details.PID = re[0][2]
		if len(re[0]) >= 4 {
			details.SerialNumber = re[0][4]
		}
		return
	}

	// Other unidentified device type
}

// setupapi based
// --------------

//sys setupDiGetClassDevs(guid *windows.GUID, enumerator *string, hwndParent uintptr, flags windows.DIGCF) (set windows.DevInfo, err error) = setupapi.SetupDiGetClassDevsW
//sys setupDiDestroyDeviceInfoList(set windows.DevInfo) (err error) = setupapi.SetupDiDestroyDeviceInfoList
//sys setupDiEnumDeviceInfo(set windows.DevInfo, index uint32, info *devInfoData) (err error) = setupapi.SetupDiEnumDeviceInfo
//sys setupDiGetDeviceInstanceId(set windows.DevInfo, devInfo *devInfoData, devInstanceId unsafe.Pointer, devInstanceIdSize uint32, requiredSize *uint32) (err error) = setupapi.SetupDiGetDeviceInstanceIdW
//sys setupDiOpenDevRegKey(set windows.DevInfo, devInfo *devInfoData, scope windows.DICS_FLAG, hwProfile uint32, keyType windows.DIREG, samDesired uint32) (hkey syscall.Handle, err error) = setupapi.SetupDiOpenDevRegKey
//sys setupDiGetDeviceRegistryProperty(set windows.DevInfo, devInfo *devInfoData, property windows.SPDRP, propertyType *uint32, outValue *byte, bufSize uint32, reqSize *uint32) (res bool) = setupapi.SetupDiGetDeviceRegistryPropertyW

//sys cmGetParent(outParentDev *windows.DEVINST, dev windows.DEVINST, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_Parent
//sys cmGetDeviceIDSize(outLen *uint32, dev windows.DEVINST, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_Device_ID_Size
//sys cmGetDeviceID(dev windows.DEVINST, buffer unsafe.Pointer, bufferSize uint32, flags uint32) (err cmError) = cfgmgr32.CM_Get_Device_IDW
//sys cmMapCrToWin32Err(cmErr cmError, defaultErr uint32) (err uint32) = cfgmgr32.CM_MapCrToWin32Err
//sys cmGetDevNodeRegistryProperty(dev windows.DEVINST, property uint32, regDataType *uint32, buffer *byte, bufferLen *uint32, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_DevNode_Registry_PropertyW

type cmError uint32

// https://msdn.microsoft.com/en-us/library/windows/hardware/ff552344(v=vs.85).aspx
type devInfoData struct {
	size     uint32
	guid     windows.GUID
	devInst  windows.DEVINST
	reserved uintptr
}

func cmConvertError(cmErr cmError) error {
	if cmErr == 0 {
		return nil
	}
	winErr := cmMapCrToWin32Err(cmErr, 0)
	return fmt.Errorf("error %d", winErr)
}

func getParent(dev windows.DEVINST) (windows.DEVINST, error) {
	var res windows.DEVINST
	cmErr := cmGetParent(&res, dev, 0)
	return res, cmConvertError(cmErr)
}

func getDeviceID(dev windows.DEVINST) (string, error) {
	var size uint32
	cmErr := cmGetDeviceIDSize(&size, dev, 0)
	if err := cmConvertError(cmErr); err != nil {
		return "", err
	}
	buff := make([]uint16, size)
	cmErr = cmGetDeviceID(dev, unsafe.Pointer(&buff[0]), uint32(len(buff)), 0)
	if err := cmConvertError(cmErr); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buff[:]), nil
}

type deviceInfo struct {
	set  windows.DevInfo
	data devInfoData
}

func getDeviceInfo(set windows.DevInfo, index int) (*deviceInfo, error) {
	result := &deviceInfo{set: set}

	result.data.size = uint32(unsafe.Sizeof(result.data))
	err := setupDiEnumDeviceInfo(set, uint32(index), &result.data)
	return result, err
}

func (dev *deviceInfo) getInstanceID() (string, error) {
	n := uint32(0)
	setupDiGetDeviceInstanceId(dev.set, &dev.data, nil, 0, &n)
	buff := make([]uint16, n)
	if err := setupDiGetDeviceInstanceId(dev.set, &dev.data, unsafe.Pointer(&buff[0]), uint32(len(buff)), &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buff[:]), nil
}

func (dev *deviceInfo) openDevRegKey(scope windows.DICS_FLAG, hwProfile uint32, keyType windows.DIREG, samDesired uint32) (syscall.Handle, error) {
	return setupDiOpenDevRegKey(dev.set, &dev.data, scope, hwProfile, keyType, samDesired)
}

func nativeGetDetailedPortsList() ([]*PortDetails, error) {
	guids, err := windows.SetupDiClassGuidsFromNameEx("Ports", "")
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}

	var res []*PortDetails
	for _, g := range guids {
		devsSet, err := setupDiGetClassDevs(&g, nil, 0, windows.DIGCF_PRESENT)
		if err != nil {
			return nil, &PortEnumerationError{causedBy: err}
		}
		defer setupDiDestroyDeviceInfoList(devsSet)

		for i := 0; ; i++ {
			device, err := getDeviceInfo(devsSet, i)
			if err != nil {
				break
			}
			details := &PortDetails{}
			portName, err := retrievePortNameFromDevInfo(device)
			if err != nil {
				continue
			}
			if len(portName) < 3 || portName[0:3] != "COM" {
				// Accept only COM ports
				continue
			}
			details.Name = portName

			if err := retrievePortDetailsFromDevInfo(device, details); err != nil {
				return nil, &PortEnumerationError{causedBy: err}
			}
			res = append(res, details)
		}
	}
	return res, nil
}

func retrievePortNameFromDevInfo(device *deviceInfo) (string, error) {
	h, err := device.openDevRegKey(windows.DICS_FLAG_GLOBAL, 0, windows.DIREG_DEV, windows.KEY_READ)
	if err != nil {
		return "", err
	}
	defer syscall.RegCloseKey(h)

	var name [1024]uint16
	nameP := (*byte)(unsafe.Pointer(&name[0]))
	nameSize := uint32(len(name) * 2)
	if err := syscall.RegQueryValueEx(h, syscall.StringToUTF16Ptr("PortName"), nil, nil, nameP, &nameSize); err != nil {
		return "", err
	}
	return syscall.UTF16ToString(name[:]), nil
}

func retrievePortDetailsFromDevInfo(device *deviceInfo, details *PortDetails) error {
	deviceID, err := device.getInstanceID()
	if err != nil {
		return err
	}
	parseDeviceID(deviceID, details)

	// On composite USB devices the serial number is usually reported on the parent
	// device, so let's navigate up one level and see if we can get this information
	if details.IsUSB && details.SerialNumber == "" {
		if parentInfo, err := getParent(device.data.devInst); err == nil {
			if parentDeviceID, err := getDeviceID(parentInfo); err == nil {
				d := &PortDetails{}
				parseDeviceID(parentDeviceID, d)
				if details.VID == d.VID && details.PID == d.PID {
					details.SerialNumber = d.SerialNumber
				}
			}
		}
	}

	/*	spdrpDeviceDesc returns a generic name, e.g.: "CDC-ACM", which will be the same for 2 identical devices attached
		while spdrpFriendlyName returns a specific name, e.g.: "CDC-ACM (COM44)",
		the result of spdrpFriendlyName is therefore unique and suitable as an alternative string to for a port choice */
	n := uint32(0)
	setupDiGetDeviceRegistryProperty(device.set, &device.data, windows.SPDRP_FRIENDLYNAME, nil, nil, 0, &n)
	if n > 0 {
		buff := make([]uint16, n*2)
		buffP := (*byte)(unsafe.Pointer(&buff[0]))
		if setupDiGetDeviceRegistryProperty(device.set, &device.data, windows.SPDRP_FRIENDLYNAME, nil, buffP, n, &n) {
			details.Product = syscall.UTF16ToString(buff[:])
		}
	}

	if details.IsUSB {
		details.Configuration = retrieveConfigurationViaHubIOCTL(device)
	}

	return nil
}

// ---- USB hub IOCTL path for iConfiguration retrieval ----
//
// Mirrors the approach used by USBView (Windows-driver-samples/usb/usbview/enum.c):
//   1. Enumerate all USB hub device interfaces.
//   2. For each hub, iterate its ports and match by driver key name.
//   3. On match, read the USB Configuration Descriptor to get iConfiguration index.
//   4. Fetch the String Descriptor at that index (language 0x0409, English).

// GUID_DEVINTERFACE_USB_HUB = {f18a0e88-c30c-11d0-8815-00a0c906bed8}
var guidDevInterfaceUSBHub = windows.GUID{
	Data1: 0xf18a0e88,
	Data2: 0xc30c,
	Data3: 0x11d0,
	Data4: [8]byte{0x88, 0x15, 0x00, 0xa0, 0xc9, 0x06, 0xbe, 0xd8},
}

const (
	// CTL_CODE(FILE_DEVICE_USB=0x22, fn, METHOD_BUFFERED=0, FILE_ANY_ACCESS=0)
	ioctlUsbGetNodeInformation              = 0x220404 // fn=0x101
	ioctlUsbGetNodeConnectionDriverkeyName  = 0x220420 // fn=0x108
	ioctlUsbGetDescriptorFromNodeConnection = 0x220410 // fn=0x104

	usbConfigurationDescriptorType = 0x02
	usbStringDescriptorType        = 0x03
	maximumUsbStringLength         = 255
	langIDEnglishUS                = 0x0409
)

// usbHubInfoHeader overlaps the beginning of USB_NODE_INFORMATION for hubs
// to extract bNumberOfPorts. Layout (no padding):
//
//	[0..3]  NodeType (uint32)
//	[4]     bDescriptorLength (uint8)
//	[5]     bDescriptorType (uint8)
//	[6]     bNumberOfPorts (uint8)
type usbHubInfoHeader struct {
	NodeType         uint32
	DescriptorLength uint8
	DescriptorType   uint8
	NumberOfPorts    uint8
}

// usbNodeConnectionDriverkeyName corresponds to USB_NODE_CONNECTION_DRIVERKEY_NAME.
// The DriverKeyName array is variable length; we allocate extra bytes at runtime.
type usbNodeConnectionDriverkeyName struct {
	ConnectionIndex uint32
	ActualLength    uint32
	DriverKeyName   [1]uint16 // variable; allocate more bytes as needed
}

// usbDescriptorRequest corresponds to USB_DESCRIPTOR_REQUEST.
// SetupPacket fields follow ConnectionIndex directly (no padding in the C struct).
type usbDescriptorRequest struct {
	ConnectionIndex uint32
	BmRequest       uint8
	BRequest        uint8
	WValue          uint16
	WIndex          uint16
	WLength         uint16
}

// usbConfigurationDescriptor is the 9-byte USB Configuration Descriptor header.
type usbConfigurationDescriptor struct {
	BLength             uint8
	BDescriptorType     uint8
	WTotalLength        uint16
	BNumInterfaces      uint8
	BConfigurationValue uint8
	IConfiguration      uint8
	BmAttributes        uint8
	MaxPower            uint8
}

// usbStringDescriptorHeader is the 2-byte prefix of a USB String Descriptor.
type usbStringDescriptorHeader struct {
	BLength         uint8
	BDescriptorType uint8
}

// cmDrpDriver is the CM_DRP_DRIVER property code (1-based, unlike SPDRP which is 0-based).
const cmDrpDriver = 0xA

// devInstanceGetDriverKey retrieves the SPDRP_DRIVER equivalent for a raw windows.DEVINST
// using CM_Get_DevNode_Registry_PropertyW.
func devInstanceGetDriverKey(inst windows.DEVINST) string {
	var size uint32
	cmGetDevNodeRegistryProperty(inst, cmDrpDriver, nil, nil, &size, 0)
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := cmConvertError(cmGetDevNodeRegistryProperty(inst, cmDrpDriver, nil, &buf[0], &size, 0)); err != nil {
		return ""
	}
	nChars := size / 2
	if nChars == 0 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[0])), nChars))
}

// findUSBPortDriverKey walks up the devnode tree from inst to find the device
// directly attached to a USB hub port (instance ID starts with "USB\") and
// returns its driver key. This is the key that IOCTL_USB_GET_NODE_CONNECTION_DRIVERKEY_NAME
// returns. For single-function USB serial devices the COM port IS that device;
// for composite USB devices the COM port is a child and we need the parent.
func findUSBPortDriverKey(inst windows.DEVINST) string {
	for i := 0; i < 5; i++ {
		id, err := getDeviceID(inst)
		if err != nil {
			return ""
		}
		uid := strings.ToUpper(id)
		// Skip composite device interfaces (e.g. USB\VID_...&MI_00\...).
		// The device directly on the hub port has no &MI_ in its instance ID.
		if strings.HasPrefix(uid, "USB\\") && !strings.Contains(uid, "&MI_") {
			if dk := devInstanceGetDriverKey(inst); dk != "" {
				return dk
			}
		}
		parent, err := getParent(inst)
		if err != nil {
			return ""
		}
		inst = parent
	}
	return ""
}

// retrieveConfigurationViaHubIOCTL looks up the USB configuration name string
// for device by matching its driver key against hub port driver keys, then
// reading the configuration descriptor and string descriptor via hub IOCTLs.
func retrieveConfigurationViaHubIOCTL(device *deviceInfo) string {
	// Find the driver key of the USB device directly attached to the hub port.
	// For composite USB devices the COM port is a child; we need the parent's key.
	targetDriverKey := findUSBPortDriverKey(device.data.devInst)
	if targetDriverKey == "" {
		return ""
	}

	// Enumerate all USB hub device interface paths.
	// Passing "" as deviceID asks for all interfaces of this class.
	hubPaths, err := windows.CM_Get_Device_Interface_List("", &guidDevInterfaceUSBHub, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil {
		return ""
	}

	for _, hubPath := range hubPaths {
		if conf := retrieveConfigFromHub(hubPath, targetDriverKey); conf != "" {
			return conf
		}
	}
	return ""
}

// retrieveConfigFromHub opens a single hub and scans its ports for targetDriverKey.
func retrieveConfigFromHub(hubPath, targetDriverKey string) string {
	hubPathPtr, err := syscall.UTF16PtrFromString(hubPath)
	if err != nil {
		return ""
	}
	hHub, err := windows.CreateFile(
		hubPathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(hHub)

	// Ask hub for its number of ports via IOCTL_USB_GET_NODE_INFORMATION.
	var hubInfo usbHubInfoHeader
	var bytesReturned uint32
	err = windows.DeviceIoControl(
		hHub,
		ioctlUsbGetNodeInformation,
		(*byte)(unsafe.Pointer(&hubInfo)),
		uint32(unsafe.Sizeof(hubInfo)),
		(*byte)(unsafe.Pointer(&hubInfo)),
		uint32(unsafe.Sizeof(hubInfo)),
		&bytesReturned,
		nil,
	)
	numPorts := uint32(hubInfo.NumberOfPorts)
	if err != nil || numPorts == 0 {
		// Fall back to scanning a reasonable maximum.
		numPorts = 16
	}

	for portIndex := uint32(1); portIndex <= numPorts; portIndex++ {
		driverKey, err := hubPortDriverKey(hHub, portIndex)
		if err != nil {
			continue
		}
		if strings.EqualFold(driverKey, targetDriverKey) {
			iConfIdx, err := hubConfigDescriptorIConfiguration(hHub, portIndex)
			if err != nil || iConfIdx == 0 {
				return ""
			}
			return hubStringDescriptor(hHub, portIndex, iConfIdx, langIDEnglishUS)
		}
	}
	return ""
}

// hubPortDriverKey retrieves the driver key name of the device at portIndex on hHub
// using IOCTL_USB_GET_NODE_CONNECTION_DRIVERKEY_NAME (mirrors GetDriverKeyName in enum.c).
func hubPortDriverKey(hHub windows.Handle, portIndex uint32) (string, error) {
	// First call: get the required ActualLength.
	var req usbNodeConnectionDriverkeyName
	req.ConnectionIndex = portIndex
	var nBytes uint32
	_ = windows.DeviceIoControl(
		hHub,
		ioctlUsbGetNodeConnectionDriverkeyName,
		(*byte)(unsafe.Pointer(&req)),
		uint32(unsafe.Sizeof(req)),
		(*byte)(unsafe.Pointer(&req)),
		uint32(unsafe.Sizeof(req)),
		&nBytes,
		nil,
	)
	if req.ActualLength <= uint32(unsafe.Sizeof(req)) {
		return "", fmt.Errorf("no driver key")
	}

	// Second call: allocate a buffer large enough for the full name.
	buf := make([]byte, req.ActualLength)
	reqFull := (*usbNodeConnectionDriverkeyName)(unsafe.Pointer(&buf[0]))
	reqFull.ConnectionIndex = portIndex
	err := windows.DeviceIoControl(
		hHub,
		ioctlUsbGetNodeConnectionDriverkeyName,
		&buf[0],
		uint32(len(buf)),
		&buf[0],
		uint32(len(buf)),
		&nBytes,
		nil,
	)
	if err != nil {
		return "", err
	}

	// DriverKeyName starts at offset 8 (after ConnectionIndex + ActualLength).
	nameStart := 8
	if len(buf) <= nameStart+1 {
		return "", fmt.Errorf("buffer too small")
	}
	nameSlice := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[nameStart])), (len(buf)-nameStart)/2)
	return windows.UTF16ToString(nameSlice), nil
}

// hubConfigDescriptorIConfiguration retrieves the iConfiguration index byte from the
// USB Configuration Descriptor for the device at portIndex (mirrors GetConfigDescriptor).
func hubConfigDescriptorIConfiguration(hHub windows.Handle, portIndex uint32) (uint8, error) {
	// First pass: use a fixed-size buffer for the config descriptor header.
	headerSize := uint32(unsafe.Sizeof(usbDescriptorRequest{}) + unsafe.Sizeof(usbConfigurationDescriptor{}))
	buf := make([]byte, headerSize)

	req := (*usbDescriptorRequest)(unsafe.Pointer(&buf[0]))
	req.ConnectionIndex = portIndex
	req.WValue = usbConfigurationDescriptorType << 8 // descriptor type in high byte, index 0 in low byte
	req.WLength = uint16(unsafe.Sizeof(usbConfigurationDescriptor{}))

	var nBytes uint32
	err := windows.DeviceIoControl(
		hHub,
		ioctlUsbGetDescriptorFromNodeConnection,
		&buf[0],
		headerSize,
		&buf[0],
		headerSize,
		&nBytes,
		nil,
	)
	if err != nil || nBytes < headerSize {
		return 0, fmt.Errorf("config descriptor IOCTL failed: %w", err)
	}

	reqSize := uint32(unsafe.Sizeof(usbDescriptorRequest{}))
	confDesc := (*usbConfigurationDescriptor)(unsafe.Pointer(&buf[reqSize]))
	if confDesc.BDescriptorType != usbConfigurationDescriptorType {
		return 0, fmt.Errorf("unexpected descriptor type %d", confDesc.BDescriptorType)
	}
	return confDesc.IConfiguration, nil
}

// hubStringDescriptor fetches the USB String Descriptor at descriptorIndex / languageID
// and returns the decoded UTF-16 string (mirrors GetStringDescriptor in enum.c).
func hubStringDescriptor(hHub windows.Handle, portIndex uint32, descriptorIndex uint8, languageID uint16) string {
	reqHeaderSize := uint32(unsafe.Sizeof(usbDescriptorRequest{}))
	totalSize := reqHeaderSize + uint32(maximumUsbStringLength) + 2 // +2 for safety
	buf := make([]byte, totalSize)

	req := (*usbDescriptorRequest)(unsafe.Pointer(&buf[0]))
	req.ConnectionIndex = portIndex
	req.WValue = (usbStringDescriptorType << 8) | uint16(descriptorIndex)
	req.WIndex = languageID
	req.WLength = uint16(maximumUsbStringLength)

	var nBytes uint32
	err := windows.DeviceIoControl(
		hHub,
		ioctlUsbGetDescriptorFromNodeConnection,
		&buf[0],
		totalSize,
		&buf[0],
		totalSize,
		&nBytes,
		nil,
	)
	if err != nil || nBytes < reqHeaderSize+2 {
		return ""
	}

	strDesc := (*usbStringDescriptorHeader)(unsafe.Pointer(&buf[reqHeaderSize]))
	if strDesc.BDescriptorType != usbStringDescriptorType {
		return ""
	}
	strLen := uint32(strDesc.BLength)
	if strLen < 2 || strLen%2 != 0 {
		return ""
	}
	// The string data (UTF-16LE) starts 2 bytes after the header.
	strDataOffset := reqHeaderSize + 2
	if uint32(nBytes) < strDataOffset+strLen-2 {
		return ""
	}
	numChars := (strLen - 2) / 2
	if numChars == 0 {
		return ""
	}
	chars := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[strDataOffset])), numChars)
	return windows.UTF16ToString(chars)
}
