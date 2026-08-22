//
// Copyright 2014-2026 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.bug.st/serial"
)

func nativeGetDetailedPortsList(shouldProbeUSB func(vid, pid string) bool) ([]*PortDetails, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}

	var result []*PortDetails

	devices, err := getFreeBSDUSBDevices()
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}

	for _, port := range ports {
		name := filepath.Base(port)

		if !strings.HasPrefix(name, "cuaU") && !strings.HasPrefix(name, "ttyU") {
			continue
		}

		ttyName := strings.TrimPrefix(name, "cua")
		ttyName = strings.TrimPrefix(ttyName, "tty")
		info, ok := devices[ttyName]
		if !ok {
			result = append(result, &PortDetails{
				Name:  port,
				IsUSB: true,
			})
			continue
		}

		details := &PortDetails{
			Name:          port,
			IsUSB:         true,
			VID:           info.VID,
			PID:           info.PID,
			SerialNumber:  info.SerialNumber,
			Configuration: info.Configuration,
			Manufacturer:  info.Manufacturer,
			Product:       info.Product,
		}

		if shouldProbeUSB(details.VID, details.PID) && info.ugen != "" {
			probed, err := probeUSBDevice(info.ugen)
			if err != nil {
				return nil, &PortEnumerationError{causedBy: err}
			}

			if probed.Manufacturer != "" {
				details.Manufacturer = probed.Manufacturer
			}
			if probed.Product != "" {
				details.Product = probed.Product
			}
			if probed.SerialNumber != "" {
				details.SerialNumber = probed.SerialNumber
			}
			if probed.Configuration != "" {
				details.Configuration = probed.Configuration
			}
		}

		result = append(result, details)
	}

	return result, nil
}

type freeBSDUSBDevice struct {
	VID           string
	PID           string
	SerialNumber  string
	ugen          string
	Manufacturer  string
	Product       string
	Configuration string
}

func getFreeBSDUSBDevices() (map[string]*freeBSDUSBDevice, error) {
	out, err := exec.Command("sysctl", "-a").Output()
	if err != nil {
		return nil, fmt.Errorf("running sysctl: %w", err)
	}

	type rawDevice struct {
		ttyName      string
		vendor       string
		product      string
		serialNumber string
		ugen         string
	}
	byNode := make(map[string]*rawDevice)

	scanner := bufio.NewScanner(strings.NewReader(string(out)))

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "dev.") {
			continue
		}

		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		var node string
		switch {
		case strings.HasSuffix(key, ".%pnpinfo"):
			node = strings.TrimSuffix(key, ".%pnpinfo")
		case strings.HasSuffix(key, ".%location"):
			node = strings.TrimSuffix(key, ".%location")
		default:
			continue
		}

		dev, ok := byNode[node]
		if !ok {
			dev = &rawDevice{}
			byNode[node] = dev
		}

		if strings.HasSuffix(key, ".%pnpinfo") {
			if ttyName := parseField(value, "ttyname"); ttyName != "" {
				dev.ttyName = ttyName
			}
			if vendor := parseField(value, "vendor"); vendor != "" {
				dev.vendor = vendor
			}
			if product := parseField(value, "product"); product != "" {
				dev.product = product
			}
			dev.serialNumber = parseField(value, "sernum")
		} else {
			dev.ugen = parseField(value, "ugen")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	result := make(map[string]*freeBSDUSBDevice)

	for _, dev := range byNode {
		if dev.ttyName == "" || dev.vendor == "" || dev.product == "" {
			continue
		}

		result[dev.ttyName] = &freeBSDUSBDevice{
			VID:          normalizeHex(dev.vendor),
			PID:          normalizeHex(dev.product),
			SerialNumber: dev.serialNumber,
			ugen:         dev.ugen,
		}
	}

	return result, nil
}

func parseField(s, field string) string {
	prefix := field + "="

	for _, token := range strings.Fields(s) {
		if strings.HasPrefix(token, prefix) {
			value := strings.TrimPrefix(token, prefix)
			value = strings.Trim(value, `"`)
			return value
		}
	}

	return ""
}

func normalizeHex(value string) string {
	value = strings.TrimPrefix(strings.ToLower(value), "0x")

	if n, err := strconv.ParseUint(value, 16, 16); err == nil {
		return fmt.Sprintf("%04X", n)
	}

	return strings.ToUpper(value)
}

type probedUSBInfo struct {
	Manufacturer  string
	Product       string
	SerialNumber  string
	Configuration string
}

var manufacturerRE = regexp.MustCompile(`(?m)^\s*iManufacturer\s*=.*<([^>]+)>`)
var productRE = regexp.MustCompile(`(?m)^\s*iProduct\s*=.*<([^>]+)>`)
var serialRE = regexp.MustCompile(`(?m)^\s*iSerialNumber\s*=.*<([^>]+)>`)
var configurationRE = regexp.MustCompile(`(?m)^\s*iConfiguration\s*=.*<([^>]+)>`)

func probeUSBDevice(ugen string) (*probedUSBInfo, error) {
	out, err := exec.Command(
		"usbconfig",
		"-d",
		ugen,
		"dump_device_desc",
	).CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("usbconfig %s: %w: %s", ugen, err, strings.TrimSpace(string(out)))
	}

	text := string(out)

	result := &probedUSBInfo{}

	if match := manufacturerRE.FindStringSubmatch(text); len(match) > 1 {
		if !strings.Contains(match[1], "no string") {
			result.Manufacturer = match[1]
		}
	}

	if match := productRE.FindStringSubmatch(text); len(match) > 1 {
		if !strings.Contains(match[1], "no string") {
			result.Product = match[1]
		}
	}

	if match := serialRE.FindStringSubmatch(text); len(match) > 1 {
		if !strings.Contains(match[1], "no string") {
			result.SerialNumber = match[1]
		}
	}

	configOut, err := exec.Command(
		"usbconfig",
		"-d",
		ugen,
		"dump_curr_config_desc",
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("usbconfig %s dump_curr_config_desc: %w: %s", ugen, err, strings.TrimSpace(string(configOut)))
	}

	if match := configurationRE.FindStringSubmatch(string(configOut)); len(match) > 1 {
		if !strings.Contains(match[1], "no string") {
			result.Configuration = match[1]
		}
	}

	return result, nil
}
