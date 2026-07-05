package decimal_test

import (
	"fmt"

	"github.com/v8fg/kit4go/decimal"
)

func ExampleParse() {
	price, _ := decimal.Parse("12.50", 2)
	qty := decimal.FromInt(3)
	total := price.Mul(qty.Unscaled().Int64())
	fmt.Println(total)
	// Output: 37.50
}
