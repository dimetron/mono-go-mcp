package monoapi

import (
	"strconv"
	"time"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func nowUnix() int64 { return time.Now().Unix() }
