package bimap_test

import (
	"fmt"

	"github.com/v8fg/kit4go/bimap"
)

func ExampleNew() {
	bm := bimap.New[int, string]()
	bm.Insert(200, "OK")
	bm.Insert(404, "Not Found")

	status, _ := bm.Get(404)
	fmt.Println("404 →", status)

	code, _ := bm.GetKey("OK")
	fmt.Println("OK →", code)

	// Output:
	// 404 → Not Found
	// OK → 200
}
