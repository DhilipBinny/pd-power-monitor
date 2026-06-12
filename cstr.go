//go:build linux || darwin

package main

/*
#include <stdlib.h>
*/
import "C"
import "unsafe"

// withCStr runs fn with a C copy of s and frees it afterwards.
// Shared by both platform trays.
func withCStr(s string, fn func(*C.char)) {
	cs := C.CString(s)
	fn(cs)
	C.free(unsafe.Pointer(cs))
}
