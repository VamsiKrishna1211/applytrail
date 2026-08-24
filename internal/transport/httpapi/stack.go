package httpapi

import "runtime"

// stack captures the current goroutine's stack, growing the buffer until it
// fits rather than truncating at a fixed size.
func stack() []byte {
	buf := make([]byte, 8<<10)
	for {
		n := runtime.Stack(buf, false)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}
