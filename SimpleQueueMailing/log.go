// Copyright (c) 2026 Mattia Cabrini
// SPDX-License-Identifier: MIT

package SimpleQueueMailing

import (
	"github.com/mattia-cabrini/go-utility"
	"time"
)

const logDateFormat = "2006-01-02 15:04:05"

// logf wraps utility.Logf, prefixing every message with the current local date
// and time in ISO format (YYYY-MM-DD hh:mm:ss).
func logf(level utility.LogLevel, format string, args ...any) {
	args = append([]any{time.Now().Format(logDateFormat)}, args...)
	utility.Logf(level, "%s "+format, args...)
}
