package bit_test

import (
	"fmt"

	"github.com/v8fg/kit4go/bit"
)

func ExampleCountOneBit() {
	start := -5
	end := 10
	for i := start; i < end; i++ {
		ret := bit.CountOneBit(i)
		fmt.Printf("[ExampleCountOneBit] num:%v, one bit count:%v\n", i, ret)
	}

	// output:
	// [ExampleCountOneBit] num:-5, one bit count:63
	// [ExampleCountOneBit] num:-4, one bit count:62
	// [ExampleCountOneBit] num:-3, one bit count:63
	// [ExampleCountOneBit] num:-2, one bit count:63
	// [ExampleCountOneBit] num:-1, one bit count:64
	// [ExampleCountOneBit] num:0, one bit count:0
	// [ExampleCountOneBit] num:1, one bit count:1
	// [ExampleCountOneBit] num:2, one bit count:1
	// [ExampleCountOneBit] num:3, one bit count:2
	// [ExampleCountOneBit] num:4, one bit count:1
	// [ExampleCountOneBit] num:5, one bit count:2
	// [ExampleCountOneBit] num:6, one bit count:2
	// [ExampleCountOneBit] num:7, one bit count:3
	// [ExampleCountOneBit] num:8, one bit count:1
	// [ExampleCountOneBit] num:9, one bit count:2

}

func ExampleIsPowerOfTwo() {
	start := -5
	end := 10
	for i := start; i < end; i++ {
		ret := bit.IsPowerOfTwo(i)
		fmt.Printf("[ExampleIsPowerOfTwo] IsPowerOfTwo(%2d)=%v\n", i, ret)
	}
	// output:
	// [ExampleIsPowerOfTwo] IsPowerOfTwo(-5)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo(-4)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo(-3)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo(-2)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo(-1)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 0)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 1)=true
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 2)=true
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 3)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 4)=true
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 5)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 6)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 7)=false
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 8)=true
	// [ExampleIsPowerOfTwo] IsPowerOfTwo( 9)=false

}

func ExampleLeftOneBitNum() {
	start := -5
	end := 10
	for i := start; i < end; i++ {
		ret := bit.LeftOneBitNum(i)
		fmt.Printf("[ExampleLeftOneBitNum] LeftOneBitNum(%2d)=%v\n", i, ret)
	}
	// output:
	// [ExampleLeftOneBitNum] LeftOneBitNum(-5)=0
	// [ExampleLeftOneBitNum] LeftOneBitNum(-4)=0
	// [ExampleLeftOneBitNum] LeftOneBitNum(-3)=0
	// [ExampleLeftOneBitNum] LeftOneBitNum(-2)=0
	// [ExampleLeftOneBitNum] LeftOneBitNum(-1)=0
	// [ExampleLeftOneBitNum] LeftOneBitNum( 0)=0
	// [ExampleLeftOneBitNum] LeftOneBitNum( 1)=1
	// [ExampleLeftOneBitNum] LeftOneBitNum( 2)=2
	// [ExampleLeftOneBitNum] LeftOneBitNum( 3)=2
	// [ExampleLeftOneBitNum] LeftOneBitNum( 4)=4
	// [ExampleLeftOneBitNum] LeftOneBitNum( 5)=4
	// [ExampleLeftOneBitNum] LeftOneBitNum( 6)=4
	// [ExampleLeftOneBitNum] LeftOneBitNum( 7)=4
	// [ExampleLeftOneBitNum] LeftOneBitNum( 8)=8
	// [ExampleLeftOneBitNum] LeftOneBitNum( 9)=8

}

func ExampleRightOneBitNum() {
	start := -5
	end := 10
	for i := start; i < end; i++ {
		ret := bit.RightOneBitNum(i)
		fmt.Printf("[ExampleRightOneBitNum] RightOneBitNum(%2d)=%v\n", i, ret)
	}
	// output:
	// [ExampleRightOneBitNum] RightOneBitNum(-5)=1
	// [ExampleRightOneBitNum] RightOneBitNum(-4)=4
	// [ExampleRightOneBitNum] RightOneBitNum(-3)=1
	// [ExampleRightOneBitNum] RightOneBitNum(-2)=2
	// [ExampleRightOneBitNum] RightOneBitNum(-1)=1
	// [ExampleRightOneBitNum] RightOneBitNum( 0)=0
	// [ExampleRightOneBitNum] RightOneBitNum( 1)=1
	// [ExampleRightOneBitNum] RightOneBitNum( 2)=2
	// [ExampleRightOneBitNum] RightOneBitNum( 3)=1
	// [ExampleRightOneBitNum] RightOneBitNum( 4)=4
	// [ExampleRightOneBitNum] RightOneBitNum( 5)=1
	// [ExampleRightOneBitNum] RightOneBitNum( 6)=2
	// [ExampleRightOneBitNum] RightOneBitNum( 7)=1
	// [ExampleRightOneBitNum] RightOneBitNum( 8)=8
	// [ExampleRightOneBitNum] RightOneBitNum( 9)=1

}

func ExamplePreHighestPowerOfTwo() {
	start := -5
	end := 10
	for i := start; i < end; i++ {
		ret := bit.PreHighestPowerOfTwo(i)
		fmt.Printf("[ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo(%2d)=%v\n", i, ret)
	}
	// output:
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo(-5)=0
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo(-4)=0
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo(-3)=0
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo(-2)=0
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo(-1)=0
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 0)=0
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 1)=1
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 2)=2
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 3)=2
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 4)=4
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 5)=4
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 6)=4
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 7)=4
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 8)=8
	// [ExamplePreHighestPowerOfTwo] PreHighestPowerOfTwo( 9)=8

}

func ExampleNextHighestPowerOfTwo() {
	start := -5
	end := 10
	for i := start; i < end; i++ {
		ret := bit.NextHighestPowerOfTwo(i)
		fmt.Printf("[ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo(%2d)=%v\n", i, ret)
	}
	// output:
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo(-5)=0
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo(-4)=0
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo(-3)=0
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo(-2)=0
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo(-1)=0
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 0)=0
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 1)=1
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 2)=2
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 3)=4
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 4)=4
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 5)=8
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 6)=8
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 7)=8
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 8)=8
	// [ExampleNextHighestPowerOfTwo] NextHighestPowerOfTwo( 9)=16

}

func ExampleMaxBits() {
	start := -5
	end := 10
	for i := start; i < end; i++ {
		ret := bit.MaxBits(i)
		fmt.Printf("[ExampleMaxBits] MaxBits(%2d)=%v\n", i, ret)
	}
	// output:
	// [ExampleMaxBits] MaxBits(-5)=0
	// [ExampleMaxBits] MaxBits(-4)=0
	// [ExampleMaxBits] MaxBits(-3)=0
	// [ExampleMaxBits] MaxBits(-2)=0
	// [ExampleMaxBits] MaxBits(-1)=0
	// [ExampleMaxBits] MaxBits( 0)=0
	// [ExampleMaxBits] MaxBits( 1)=1
	// [ExampleMaxBits] MaxBits( 2)=2
	// [ExampleMaxBits] MaxBits( 3)=2
	// [ExampleMaxBits] MaxBits( 4)=3
	// [ExampleMaxBits] MaxBits( 5)=3
	// [ExampleMaxBits] MaxBits( 6)=3
	// [ExampleMaxBits] MaxBits( 7)=3
	// [ExampleMaxBits] MaxBits( 8)=4
	// [ExampleMaxBits] MaxBits( 9)=4

}
