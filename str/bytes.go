package str

import (
	"reflect"
	"unsafe"
)

// StringToBytes converts a string to a byte slice.
func StringToBytes(s string) []byte {
	// fixed:  govet  unsafeptr: possible misuse of reflect.SliceHeader
	// sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	// return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
	// 	Data: sh.Data,
	// 	Len:  sh.Len,
	// 	Cap:  sh.Len,
	// }))

	p := unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data)

	var b []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	hdr.Data = uintptr(p)
	hdr.Cap = len(s)
	hdr.Len = len(s)
	return b
}

// BytesToString converts a byte slice to a string.
func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
