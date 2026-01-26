package apiutil

import (
	"fmt"
	"time"
)

func FormatOperatingHourValue(value interface{}) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.Format("15:04")
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}
