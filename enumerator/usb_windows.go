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
		re := regexp.MustCompile("VID_(....)&PID_(....)(\\\\(\\w+)$)?").FindAllStringSubmatch(deviceID, -1)
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
		re := regexp.MustCompile("VID_(....)\\+PID_(....)(\\+(\\w+))?").FindAllStringSubmatch(deviceID, -1)
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

//sys setupDiClassGuidsFromNameInternal(class string, guid *guid, guidSize uint32, requiredSize *uint32) (err error) = setupapi.SetupDiClassGuidsFromNameW
//sys setupDiGetClassDevs(guid *guid, enumerator *string, hwndParent uintptr, flags uint32) (set devicesSet, err error) = setupapi.SetupDiGetClassDevsW
//sys setupDiDestroyDeviceInfoList(set devicesSet) (err error) = setupapi.SetupDiDestroyDeviceInfoList
//sys setupDiEnumDeviceInfo(set devicesSet, index uint32, info *devInfoData) (err error) = setupapi.SetupDiEnumDeviceInfo
//sys setupDiGetDeviceInstanceId(set devicesSet, devInfo *devInfoData, devInstanceId unsafe.Pointer, devInstanceIdSize uint32, requiredSize *uint32) (err error) = setupapi.SetupDiGetDeviceInstanceIdW
//sys setupDiOpenDevRegKey(set devicesSet, devInfo *devInfoData, scope dicsScope, hwProfile uint32, keyType uint32, samDesired regsam) (hkey syscall.Handle, err error) = setupapi.SetupDiOpenDevRegKey
//sys setupDiGetDeviceRegistryProperty(set devicesSet, devInfo *devInfoData, property deviceProperty, propertyType *uint32, outValue *byte, bufSize uint32, reqSize *uint32) (res bool) = setupapi.SetupDiGetDeviceRegistryPropertyW

//sys cmGetParent(outParentDev *devInstance, dev devInstance, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_Parent
//sys cmGetDeviceIDSize(outLen *uint32, dev devInstance, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_Device_ID_Size
//sys cmGetDeviceID(dev devInstance, buffer unsafe.Pointer, bufferSize uint32, flags uint32) (err cmError) = cfgmgr32.CM_Get_Device_IDW
//sys cmMapCrToWin32Err(cmErr cmError, defaultErr uint32) (err uint32) = cfgmgr32.CM_MapCrToWin32Err
//sys cmGetDevNodeRegistryProperty(dev devInstance, property uint32, regDataType *uint32, buffer *byte, bufferLen *uint32, flags uint32) (cmErr cmError) = cfgmgr32.CM_Get_DevNode_Registry_PropertyW

// Device registry property codes
// (Codes marked as read-only (R) may only be used for
// SetupDiGetDeviceRegistryProperty)
//
// These values should cover the same set of registry properties
// as defined by the CM_DRP codes in cfgmgr32.h.
//
// Note that SPDRP codes are zero based while CM_DRP codes are one based!
type deviceProperty uint32

const (
	spdrpDeviceDesc               deviceProperty = 0x00000000 // DeviceDesc = R/W
	spdrpHardwareID                              = 0x00000001 // HardwareID = R/W
	spdrpCompatibleIDS                           = 0x00000002 // CompatibleIDs = R/W
	spdrpUnused0                                 = 0x00000003 // Unused
	spdrpService                                 = 0x00000004 // Service = R/W
	spdrpUnused1                                 = 0x00000005 // Unused
	spdrpUnused2                                 = 0x00000006 // Unused
	spdrpClass                                   = 0x00000007 // Class = R--tied to ClassGUID
	spdrpClassGUID                               = 0x00000008 // ClassGUID = R/W
	spdrpDriver                                  = 0x00000009 // Driver = R/W
	spdrpConfigFlags                             = 0x0000000A // ConfigFlags = R/W
	spdrpMFG                                     = 0x0000000B // Mfg = R/W
	spdrpFriendlyName                            = 0x0000000C // FriendlyName = R/W
	spdrpLocationIinformation                    = 0x0000000D // LocationInformation = R/W
	spdrpPhysicalDeviceObjectName                = 0x0000000E // PhysicalDeviceObjectName = R
	spdrpCapabilities                            = 0x0000000F // Capabilities = R
	spdrpUINumber                                = 0x00000010 // UiNumber = R
	spdrpUpperFilters                            = 0x00000011 // UpperFilters = R/W
	spdrpLowerFilters                            = 0x00000012 // LowerFilters = R/W
	spdrpBusTypeGUID                             = 0x00000013 // BusTypeGUID = R
	spdrpLegacyBusType                           = 0x00000014 // LegacyBusType = R
	spdrpBusNumber                               = 0x00000015 // BusNumber = R
	spdrpEnumeratorName                          = 0x00000016 // Enumerator Name = R
	spdrpSecurity                                = 0x00000017 // Security = R/W, binary form
	spdrpSecuritySDS                             = 0x00000018 // Security = W, SDS form
	spdrpDevType                                 = 0x00000019 // Device Type = R/W
	spdrpExclusive                               = 0x0000001A // Device is exclusive-access = R/W
	spdrpCharacteristics                         = 0x0000001B // Device Characteristics = R/W
	spdrpAddress                                 = 0x0000001C // Device Address = R
	spdrpUINumberDescFormat                      = 0x0000001D // UiNumberDescFormat = R/W
	spdrpDevicePowerData                         = 0x0000001E // Device Power Data = R
	spdrpRemovalPolicy                           = 0x0000001F // Removal Policy = R
	spdrpRemovalPolicyHWDefault                  = 0x00000020 // Hardware Removal Policy = R
	spdrpRemovalPolicyOverride                   = 0x00000021 // Removal Policy Override = RW
	spdrpInstallState                            = 0x00000022 // Device Install State = R
	spdrpLocationPaths                           = 0x00000023 // Device Location Paths = R
	spdrpBaseContainerID                         = 0x00000024 // Base ContainerID = R

	spdrpMaximumProperty = 0x00000025 // Upper bound on ordinals
)

// Values specifying the scope of a device property change
type dicsScope uint32

const (
	dicsFlagGlobal          dicsScope = 0x00000001 // make change in all hardware profiles
	dicsFlagConfigSspecific           = 0x00000002 // make change in specified profile only
	dicsFlagConfigGeneral             = 0x00000004 // 1 or more hardware profile-specific
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/ms724878(v=vs.85).aspx
type regsam uint32

const (
	keyAllAccess        regsam = 0xF003F
	keyCreateLink              = 0x00020
	keyCreateSubKey            = 0x00004
	keyEnumerateSubKeys        = 0x00008
	keyExecute                 = 0x20019
	keyNotify                  = 0x00010
	keyQueryValue              = 0x00001
	keyRead                    = 0x20019
	keySetValue                = 0x00002
	keyWOW64_32key             = 0x00200
	keyWOW64_64key             = 0x00100
	keyWrite                   = 0x20006
)

// KeyType values for SetupDiCreateDevRegKey, SetupDiOpenDevRegKey, and
// SetupDiDeleteDevRegKey.
const (
	diregDev  = 0x00000001 // Open/Create/Delete device key
	diregDrv  = 0x00000002 // Open/Create/Delete driver key
	diregBoth = 0x00000004 // Delete both driver and Device key
)

// https://msdn.microsoft.com/it-it/library/windows/desktop/aa373931(v=vs.85).aspx
type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

func (g guid) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		g.data1, g.data2, g.data3,
		g.data4[0], g.data4[1], g.data4[2], g.data4[3],
		g.data4[4], g.data4[5], g.data4[6], g.data4[7])
}

func classGuidsFromName(className string) ([]guid, error) {
	// Determine the number of GUIDs for className
	n := uint32(0)
	if err := setupDiClassGuidsFromNameInternal(className, nil, 0, &n); err != nil {
		// ignore error: UIDs array size too small
	}

	res := make([]guid, n)
	err := setupDiClassGuidsFromNameInternal(className, &res[0], n, &n)
	return res, err
}

const (
	digcfDefault         = 0x00000001 // only valid with digcfDeviceInterface
	digcfPresent         = 0x00000002
	digcfAllClasses      = 0x00000004
	digcfProfile         = 0x00000008
	digcfDeviceInterface = 0x00000010
)

type devicesSet syscall.Handle

func (g *guid) getDevicesSet() (devicesSet, error) {
	return setupDiGetClassDevs(g, nil, 0, digcfPresent)
}

func (set devicesSet) destroy() {
	setupDiDestroyDeviceInfoList(set)
}

type cmError uint32

// https://msdn.microsoft.com/en-us/library/windows/hardware/ff552344(v=vs.85).aspx
type devInfoData struct {
	size     uint32
	guid     guid
	devInst  devInstance
	reserved uintptr
}

type devInstance uint32

func cmConvertError(cmErr cmError) error {
	if cmErr == 0 {
		return nil
	}
	winErr := cmMapCrToWin32Err(cmErr, 0)
	return fmt.Errorf("error %d", winErr)
}

func (dev devInstance) getParent() (devInstance, error) {
	var res devInstance
	errN := cmGetParent(&res, dev, 0)
	return res, cmConvertError(errN)
}

func (dev devInstance) GetDeviceID() (string, error) {
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
	set  devicesSet
	data devInfoData
}

func (set devicesSet) getDeviceInfo(index int) (*deviceInfo, error) {
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

func (dev *deviceInfo) openDevRegKey(scope dicsScope, hwProfile uint32, keyType uint32, samDesired regsam) (syscall.Handle, error) {
	return setupDiOpenDevRegKey(dev.set, &dev.data, scope, hwProfile, keyType, samDesired)
}

func nativeGetDetailedPortsList() ([]*PortDetails, error) {
	guids, err := classGuidsFromName("Ports")
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}

	var res []*PortDetails
	for _, g := range guids {
		devsSet, err := g.getDevicesSet()
		if err != nil {
			return nil, &PortEnumerationError{causedBy: err}
		}
		defer devsSet.destroy()

		for i := 0; ; i++ {
			device, err := devsSet.getDeviceInfo(i)
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
	h, err := device.openDevRegKey(dicsFlagGlobal, 0, diregDev, keyRead)
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
		if parentInfo, err := device.data.devInst.getParent(); err == nil {
			if parentDeviceID, err := parentInfo.GetDeviceID(); err == nil {
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
	setupDiGetDeviceRegistryProperty(device.set, &device.data, spdrpFriendlyName /* spdrpDeviceDesc */, nil, nil, 0, &n)
	if n > 0 {
		buff := make([]uint16, n*2)
		buffP := (*byte)(unsafe.Pointer(&buff[0]))
		if setupDiGetDeviceRegistryProperty(device.set, &device.data, spdrpFriendlyName /* spdrpDeviceDesc */, nil, buffP, n, &n) {
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

// devInstanceGetDriverKey retrieves the SPDRP_DRIVER equivalent for a raw devInstance
// using CM_Get_DevNode_Registry_PropertyW.
func devInstanceGetDriverKey(inst devInstance) string {
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
func findUSBPortDriverKey(inst devInstance) string {
	for i := 0; i < 5; i++ {
		id, err := inst.GetDeviceID()
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
		parent, err := inst.getParent()
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
