//go:build darwin

package main

/*
#include <stdlib.h>
*/
import "C"
import "unsafe"

// withCStr runs fn with a C copy of s and frees it afterwards.
// Only the darwin tray needs cgo; the Linux tray is pure Go (D-Bus).
func withCStr(s string, fn func(*C.char)) {
	cs := C.CString(s)
	fn(cs)
	C.free(unsafe.Pointer(cs))
}
