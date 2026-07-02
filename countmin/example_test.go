package countmin_test

import (
	"fmt"

	"github.com/v8fg/kit4go/countmin"
)

func ExampleCountMinSketch() {
	c := countmin.New(2048, 5) // width 2048, depth 5 rows
	c.AddString("creative-abc")
	c.AddString("creative-abc")
	c.AddString("creative-abc")
	fmt.Println(c.EstimateString("creative-abc"))
	// Output: 3
}
