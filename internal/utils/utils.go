// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package utils

import (
	"regexp"
)

func IsValidGUID(guidHex string) bool {
	// Regular expression to match a 32-character hexadecimal string
	re := regexp.MustCompile("^[a-fA-F0-9]{32}$")
	return re.MatchString(guidHex)
}
