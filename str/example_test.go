package str_test

import (
	"fmt"

	"github.com/v8fg/kit4go/str"
)

func ExampleEqualIgnoreCase() {
	fmt.Println(str.EqualIgnoreCase("Hello", "HELLO"))
	fmt.Println(str.ContainsAll("hello world", "hello", "world"))
	fmt.Println(str.ContainsAny("hello world", "foo", "world"))
	fmt.Println(str.IsBlank("   "))
	// Output:
	// true
	// true
	// true
	// true
}

func ExampleCamelToSnake() {
	// CamelCase -> snake_case, splitting common initialisms (TCP, UDP, ...).
	fmt.Println(str.CamelToSnake("CamelToSnake"))
	fmt.Println(str.CamelToSnake("NetProtocolTCPAndUDP"))
	// snake_case -> CamelCase, preserving known initialisms as one word.
	fmt.Println(str.SnakeToCamelWithDefaultInitialisms("net_protocol_tcp_and_udp", true))
	// Output:
	// camel_to_snake
	// net_protocol_tcp_and_udp
	// NetProtocolTCPAndUDP
}
