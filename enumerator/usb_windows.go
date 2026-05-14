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
//sys setupDiOpenDevRegKey(set windows.DevInfo, devInfo *windows.DevInfoData, scope windows.DICS_FLAG, hwProfile uint32, keyType windows.DIREG, samDesired uint32) (hkey syscall.Handle, err error) = setupapi.SetupDiOpenDevRegKey
//sys setupDiGetDeviceRegistryProperty(set windows.DevInfo, devInfo *windows.DevInfoData, property windows.SPDRP, propertyType *uint32, outValue *byte, bufSize uint32, reqSize *uint32) (res bool) = setupapi.SetupDiGetDeviceRegistryPropertyW

//sys cmGetParent(outParentDev *windows.DEVINST, dev windows.DEVINST, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_Parent
//sys cmGetDeviceIDSize(outLen *uint32, dev windows.DEVINST, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_Device_ID_Size
//sys cmGetDeviceID(dev windows.DEVINST, buffer unsafe.Pointer, bufferSize uint32, flags uint32) (err cmError) = cfgmgr32.CM_Get_Device_IDW
//sys cmMapCrToWin32Err(cmErr cmError, defaultErr uint32) (err uint32) = cfgmgr32.CM_MapCrToWin32Err
//sys cmGetDevNodeRegistryProperty(dev windows.DEVINST, property uint32, regDataType *uint32, buffer *byte, bufferLen *uint32, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_DevNode_Registry_PropertyW

type cmError uint32

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
	data *windows.DevInfoData
}

func getDeviceInfo(set windows.DevInfo, index int) (*deviceInfo, error) {
	data, err := windows.SetupDiEnumDeviceInfo(set, index)
	if err != nil {
		return nil, err
	}
	return &deviceInfo{set: set, data: data}, nil
}

func (dev *deviceInfo) getInstanceID() (string, error) {
	return windows.SetupDiGetDeviceInstanceId(dev.set, dev.data)
}

func (dev *deviceInfo) openDevRegKey(scope windows.DICS_FLAG, hwProfile uint32, keyType windows.DIREG, samDesired uint32) (syscall.Handle, error) {
	return setupDiOpenDevRegKey(dev.set, dev.data, scope, hwProfile, keyType, samDesired)
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
		if parentInfo, err := getParent(device.data.DevInst); err == nil {
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
		the result of spdrpFriendlyName is therefore unique and suitable as an alternative string for a port choice */
	n := uint32(0)
	setupDiGetDeviceRegistryProperty(device.set, device.data, windows.SPDRP_FRIENDLYNAME, nil, nil, 0, &n)
	if n > 0 {
		buff := make([]uint16, n*2)
		buffP := (*byte)(unsafe.Pointer(&buff[0]))
		if setupDiGetDeviceRegistryProperty(device.set, device.data, windows.SPDRP_FRIENDLYNAME, nil, buffP, n, &n) {
			details.Product = syscall.UTF16ToString(buff[:])
		}
	}

	if details.IsUSB {
		if hub, port, err := findUsbHubAndPortConnectedToDevice(device); err == nil {
			defer hub.Close()

			if usbDesc, err := hub.GetDeviceDescriptorForPort(port); err == nil {
				if usbDesc.IManufacturer != 0 {
					details.Manufacturer, _ = hub.GetStringDescriptorForPort(port, usbDesc.IManufacturer, langIDEnglishUS)
				}
				if usbDesc.IProduct != 0 {
					details.Product, _ = hub.GetStringDescriptorForPort(port, usbDesc.IProduct, langIDEnglishUS)
				}
			}

			configDesc, err := hub.GetConfigDescriptorForPort(port)
			if err == nil && configDesc.IConfiguration != 0 {
				details.Configuration, _ = hub.GetStringDescriptorForPort(port, configDesc.IConfiguration, langIDEnglishUS)
			}
		}
	}

	return nil
}

// ---- USB hub IOCTL path for iManufacturer/iProduct/iConfiguration retrieval ----
//
// Mirrors the approach used by USBView (Windows-driver-samples/usb/usbview/enum.c):
//   1. Enumerate all USB hub device interfaces.
//   2. For each hub, iterate its ports and match by driver key name.
//   3. On match, read the USB Device/Configuration Descriptor string indexes.
//   4. Fetch String Descriptors at those indexes (language 0x0409, English).

// GUID_DEVINTERFACE_USB_HUB = {f18a0e88-c30c-11d0-8815-00a0c906bed8}
var guidDevInterfaceUSBHub, _ = windows.GUIDFromString("{f18a0e88-c30c-11d0-8815-00a0c906bed8}")

const (
	// CTL_CODE(FILE_DEVICE_USB=0x22, fn, METHOD_BUFFERED=0, FILE_ANY_ACCESS=0)
	ioctlUsbGetNodeInformation              = 0x220404 // fn=0x101
	ioctlUsbGetNodeConnectionDriverkeyName  = 0x220420 // fn=0x108
	ioctlUsbGetDescriptorFromNodeConnection = 0x220410 // fn=0x104

	usbDeviceDescriptorType        = 0x01
	usbConfigurationDescriptorType = 0x02
	usbStringDescriptorType        = 0x03
	maximumUsbStringLength         = 255
	langIDEnglishUS                = 0x0409
)

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

// usbDeviceDescriptor is the 18-byte USB Device Descriptor.
type usbDeviceDescriptor struct {
	BLength            uint8
	BDescriptorType    uint8
	BcdUSB             uint16
	BDeviceClass       uint8
	BDeviceSubClass    uint8
	BDeviceProtocol    uint8
	BMaxPacketSize0    uint8
	IdVendor           uint16
	IdProduct          uint16
	BcdDevice          uint16
	IManufacturer      uint8
	IProduct           uint8
	ISerialNumber      uint8
	BNumConfigurations uint8
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
func findUSBPortDriverKey(inst windows.DEVINST) (string, error) {
	for range 5 {
		id, err := getDeviceID(inst)
		if err != nil {
			return "", err
		}
		uid := strings.ToUpper(id)
		// Skip composite device interfaces (e.g. USB\VID_...&MI_00\...).
		// The device directly on the hub port has no &MI_ in its instance ID.
		if strings.HasPrefix(uid, "USB\\") && !strings.Contains(uid, "&MI_") {
			if dk := devInstanceGetDriverKey(inst); dk != "" {
				return dk, nil
			}
		}
		parent, err := getParent(inst)
		if err != nil {
			return "", err
		}
		inst = parent
	}
	return "", fmt.Errorf("USB port driver key not found in devnode tree")
}

// enumerateUSBHubs enumerate all USB hub device interface paths.
func enumerateUSBHubs() ([]string, error) {
	// Passing "" as deviceID asks for all interfaces of this class.
	return windows.CM_Get_Device_Interface_List("", &guidDevInterfaceUSBHub, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
}

type usbHub windows.Handle

func openUsbHub(hubPath string) (usbHub, error) {
	hubPathPtr, err := syscall.UTF16PtrFromString(hubPath)
	if err != nil {
		return 0, err
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
		return 0, err
	}
	return usbHub(hHub), nil
}

func (hub usbHub) Close() error {
	return windows.CloseHandle(windows.Handle(hub))
}

func usbHubDeviceIoControl[T any](hub usbHub, ioctl uint32, buffer *T) (bytesReturned uint32, err error) {
	err = windows.DeviceIoControl(
		windows.Handle(hub),
		ioctl,
		(*byte)(unsafe.Pointer(buffer)), uint32(unsafe.Sizeof(*buffer)),
		(*byte)(unsafe.Pointer(buffer)), uint32(unsafe.Sizeof(*buffer)),
		&bytesReturned,
		nil,
	)
	return bytesReturned, err
}

func findUsbHubAndPortConnectedToDevice(device *deviceInfo) (usbHub, uint32, error) {
	// Find the driver key of the USB device directly attached to the hub port.
	// For composite USB devices the COM port is a child; we need the parent's key.
	targetDriverKey, err := findUSBPortDriverKey(device.data.DevInst)
	if err != nil {
		return 0, 0, err
	}

	hubPaths, _ := enumerateUSBHubs()
	for _, hubPath := range hubPaths {
		hub, err := openUsbHub(hubPath)
		if err != nil {
			continue
		}

		if port, found := hub.FindPortConnectedToDeviceKey(targetDriverKey); found {
			return hub, port, nil
		}

		hub.Close()
	}
	return 0, 0, fmt.Errorf("USB hub port connected to device not found")
}

func (hub usbHub) FindPortConnectedToDeviceKey(targetDriverKey string) (portIndex uint32, found bool) {
	ports, err := hub.GetNumberOfPorts()
	if err != nil || ports == 0 {
		// Fall back to scanning a reasonable maximum.
		ports = 16
	}
	for portIndex := uint32(1); portIndex <= ports; portIndex++ {
		driverKey, err := hub.GetPortDriverKey(portIndex)
		if err != nil {
			continue
		}
		if strings.EqualFold(driverKey, targetDriverKey) {
			return portIndex, true
		}
	}
	return 0, false
}

func (hub usbHub) GetNumberOfPorts() (uint32, error) {
	// hubInfo overlaps the beginning of USB_NODE_INFORMATION for hubs
	// to extract bNumberOfPorts.
	var hubInfo struct {
		NodeType         uint32
		DescriptorLength uint8
		DescriptorType   uint8
		NumberOfPorts    uint8
	}

	// Ask hub for its number of ports via IOCTL_USB_GET_NODE_INFORMATION.
	_, err := usbHubDeviceIoControl(hub, ioctlUsbGetNodeInformation, &hubInfo)
	return uint32(hubInfo.NumberOfPorts), err
}

// GetDeviceDescriptorForPort the USB Device Descriptor for the device at portIndex.
func (hub usbHub) GetDeviceDescriptorForPort(portIndex uint32) (usbDeviceDescriptor, error) {
	var buff struct {
		req  usbDescriptorRequest
		resp usbDeviceDescriptor
	}
	buff.req.ConnectionIndex = portIndex
	buff.req.WValue = usbDeviceDescriptorType << 8 // descriptor type in high byte, index 0 in low byte
	buff.req.WLength = uint16(unsafe.Sizeof(buff.resp))
	nBytes, err := usbHubDeviceIoControl(hub, ioctlUsbGetDescriptorFromNodeConnection, &buff)
	expectedSize := uint32(unsafe.Sizeof(buff.req) + unsafe.Sizeof(buff.resp)) // cannot use sizeof(buff) because the struct has padding
	if err != nil || nBytes < expectedSize {
		return usbDeviceDescriptor{}, fmt.Errorf("device descriptor IOCTL failed: %w", err)
	}

	resp := buff.resp
	if resp.BDescriptorType != usbDeviceDescriptorType {
		return usbDeviceDescriptor{}, fmt.Errorf("unexpected descriptor type %d", resp.BDescriptorType)
	}
	return resp, nil
}

// hubPortDriverKey retrieves the driver key name of the device at portIndex on hHub
// using IOCTL_USB_GET_NODE_CONNECTION_DRIVERKEY_NAME (mirrors GetDriverKeyName in enum.c).
func (hub usbHub) GetPortDriverKey(portIndex uint32) (string, error) {
	// The following structure corresponds to USB_NODE_CONNECTION_DRIVERKEY_NAME.
	var req struct {
		ConnectionIndex uint32
		ActualLength    uint32
		DriverKeyName   [1024]uint16 // assume max length of 1024 UTF-16 chars (2048 bytes), which is more than enough for a registry key path
	}
	req.ConnectionIndex = portIndex
	_, err := usbHubDeviceIoControl(hub, ioctlUsbGetNodeConnectionDriverkeyName, &req)
	if err != nil || req.ActualLength >= uint32(unsafe.Sizeof(req)) {
		return "", fmt.Errorf("reading driver key")
	}
	r := windows.UTF16PtrToString(&req.DriverKeyName[0])
	return r, nil
}

// hubConfigDescriptorIConfiguration retrieves the iConfiguration index byte from the
// USB Configuration Descriptor for the device at portIndex (mirrors GetConfigDescriptor).
func (hub usbHub) GetConfigDescriptorForPort(portIndex uint32) (usbConfigurationDescriptor, error) {
	var buff struct {
		req  usbDescriptorRequest
		resp usbConfigurationDescriptor
	}
	buff.req.ConnectionIndex = portIndex
	buff.req.WValue = usbConfigurationDescriptorType << 8 // descriptor type in high byte, index 0 in low byte
	buff.req.WLength = uint16(unsafe.Sizeof(buff.resp))
	nBytes, err := usbHubDeviceIoControl(hub, ioctlUsbGetDescriptorFromNodeConnection, &buff)
	if err != nil {
		return usbConfigurationDescriptor{}, fmt.Errorf("config descriptor IOCTL failed: %w", err)
	}
	expectedSize := uint32(unsafe.Sizeof(buff.req) + unsafe.Sizeof(buff.resp)) // cannot use sizeof(buff) because the struct has padding
	if nBytes < expectedSize {
		return usbConfigurationDescriptor{}, fmt.Errorf("config descriptor IOCTL returned insufficient data")
	}

	resp := buff.resp
	if resp.BDescriptorType != usbConfigurationDescriptorType {
		return usbConfigurationDescriptor{}, fmt.Errorf("unexpected descriptor type %d", resp.BDescriptorType)
	}
	return resp, nil
}

// hubStringDescriptor fetches the USB String Descriptor at descriptorIndex / languageID
// and returns the decoded UTF-16 string (mirrors GetStringDescriptor in enum.c).
func (hub usbHub) GetStringDescriptorForPort(portIndex uint32, descriptorIndex uint8, languageID uint16) (string, error) {
	var buff struct {
		req  usbDescriptorRequest
		resp struct {
			usbStringDescriptorHeader
			data [maximumUsbStringLength]uint16
		}
	}
	buff.req.ConnectionIndex = portIndex
	buff.req.WValue = (usbStringDescriptorType << 8) | uint16(descriptorIndex)
	buff.req.WIndex = languageID
	buff.req.WLength = uint16(maximumUsbStringLength)
	nBytes, err := usbHubDeviceIoControl(hub, ioctlUsbGetDescriptorFromNodeConnection, &buff)
	if err != nil {
		return "", fmt.Errorf("string descriptor IOCTL failed: %w", err)
	}
	if nBytes < uint32(unsafe.Sizeof(buff.req))+2 {
		return "", fmt.Errorf("string descriptor IOCTL returned insufficient data")
	}

	resp := buff.resp
	if resp.BDescriptorType != usbStringDescriptorType {
		return "", fmt.Errorf("unexpected descriptor type %d", resp.BDescriptorType)
	}
	strLen := uint32(resp.BLength)
	if strLen < 2 || strLen%2 != 0 {
		return "", fmt.Errorf("invalid string descriptor length: %d bytes", strLen)
	}
	numChars := (strLen - 2) / 2
	if numChars == 0 {
		return "", nil
	}
	return windows.UTF16ToString(resp.data[:numChars]), nil
}
