//
// Copyright 2014-2023 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package enumerator

import (
	"strings"

	"go.bug.st/serial"
)

func nativeGetDetailedPortsList() ([]*PortDetails, error) {
	// Retrieve the port list
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, &PortEnumerationError{causedBy: err}
	}

	var res []*PortDetails
	for _, port := range ports {
		details, err := nativeGetPortDetails(port)
		if err != nil {
			return nil, &PortEnumerationError{causedBy: err}
		}
		res = append(res, details)
	}
	return res, nil
}

func nativeGetPortDetails(portPath string) (*PortDetails, error) {
	result := &PortDetails{Name: portPath}

	if strings.Contains(result.Name, "U") {
		result.IsUSB = true
	}
	return result, nil
}
