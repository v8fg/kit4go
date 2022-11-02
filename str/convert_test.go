package str_test

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/str"
)

func TestLower(t *testing.T) {
	convey.Convey("TestLower", t, func() {
		convey.So(str.Lower("XWI88"), convey.ShouldEqual, "xwi88")
		convey.So(str.Lower("XWi88"), convey.ShouldEqual, "xwi88")
		convey.So(str.Lower("xwi88"), convey.ShouldEqual, "xwi88")
		convey.So(str.Lower("Go is the best language"), convey.ShouldEqual, "go is the best language")
	})
}

func TestQuote(t *testing.T) {
	convey.Convey("TestQuote", t, func() {
		convey.So(str.Quote("XWI88"), convey.ShouldEqual, `"XWI88"`)
		convey.So(str.Quote("XWi88"), convey.ShouldEqual, `"XWi88"`)
		convey.So(str.Quote("xwi88"), convey.ShouldEqual, `"xwi88"`)
		convey.So(str.Quote("Go is the best language"), convey.ShouldEqual, `"Go is the best language"`)
	})
}

func TestTitle(t *testing.T) {
	convey.Convey("TestTitle", t, func() {
		convey.So(str.Title("XWI88"), convey.ShouldEqual, "XWI88")
		convey.So(str.Title("XWi88"), convey.ShouldEqual, "XWI88")
		convey.So(str.Title("xwi88"), convey.ShouldEqual, "XWI88")
		convey.So(str.Title("Go is the best language"), convey.ShouldEqual, "GO IS THE BEST LANGUAGE")
	})
}

func TestUnquote(t *testing.T) {
	convey.Convey("TestUnquote", t, func() {
		convey.So(str.Unquote("xwi88"), convey.ShouldEqual, "xwi88")
		convey.So(str.Unquote(`"XWI88"`), convey.ShouldEqual, "XWI88")
		convey.So(str.Unquote(`"XWi88"`), convey.ShouldEqual, "XWi88")
		convey.So(str.Unquote(`"xwi88"`), convey.ShouldEqual, "xwi88")
		convey.So(str.Unquote(`"Go is the best language"`), convey.ShouldEqual, "Go is the best language")
	})
}

func TestUpper(t *testing.T) {
	convey.Convey("TestUpper", t, func() {
		convey.So(str.Upper("XWI88"), convey.ShouldEqual, "XWI88")
		convey.So(str.Upper("XWi88"), convey.ShouldEqual, "XWI88")
		convey.So(str.Upper("xwi88"), convey.ShouldEqual, "XWI88")
		convey.So(str.Upper("Go is the best language"), convey.ShouldEqual, "GO IS THE BEST LANGUAGE")
	})
}

func TestCamel(t *testing.T) {
	convey.Convey("TestCamel", t, func() {
		convey.So(str.Camel("", true, ' '), convey.ShouldEqual, "")
		convey.So(str.Camel(" ", true, ' '), convey.ShouldEqual, "")
		convey.So(str.Camel("go is the best language", true, ' '), convey.ShouldEqual, "GoIsTheBestLanguage")
		convey.So(str.Camel("go is the best language", false, ' '), convey.ShouldEqual, "goIsTheBestLanguage")
		convey.So(str.Camel("To.Camel.Case", false, '.'), convey.ShouldEqual, "ToCamelCase")
		convey.So(str.Camel(" to @ Camel case", true, '@'), convey.ShouldEqual, "ToCamelCase")
		convey.So(str.Camel(" to @ Camel case", false, '@'), convey.ShouldEqual, "toCamelCase")
		convey.So(str.Camel(" @to @ Camel case", false, '@'), convey.ShouldEqual, "toCamelCase")
		convey.So(str.Camel(" @to @ Camel case", true, '@'), convey.ShouldEqual, "ToCamelCase")
		convey.So(str.Camel(" @", false, '@'), convey.ShouldEqual, "")
		convey.So(str.Camel("go_is_the_best language", true, ' ', '_'), convey.ShouldEqual, "GoIsTheBestLanguage")
		convey.So(str.Camel("go_is_the_best language", true, '_'), convey.ShouldEqual, "GoIsTheBestLanguage")
		convey.So(str.Camel("go_is_the_best @language", true, '_', '@'), convey.ShouldEqual, "GoIsTheBestLanguage")
	})
}

func TestSnakeToCamel(t *testing.T) {
	convey.Convey("TestSnakeToCamel", t, func() {
		convey.So(str.SnakeToCamel("go_is_the_best_language", false), convey.ShouldEqual, "goIsTheBestLanguage")
		convey.So(str.SnakeToCamel("go_is_the_best_language", true), convey.ShouldEqual, "GoIsTheBestLanguage")
		convey.So(str.SnakeToCamel("http_with_tcp_and_udp", true), convey.ShouldEqual, "HttpWithTcpAndUdp")
	})
}

func TestCamelToSnake(t *testing.T) {
	convey.Convey("TestCamelToSnake", t, func() {
		convey.So(str.CamelToSnake("camelToSnake"), convey.ShouldEqual, "camel_to_snake")
		convey.So(str.CamelToSnake("CamelToSnake"), convey.ShouldEqual, "camel_to_snake")
		convey.So(str.CamelToSnake("jsonFormat"), convey.ShouldEqual, "json_format")
		convey.So(str.CamelToSnake("NetProtocolTCPAndUDP"), convey.ShouldEqual, "net_protocol_tcp_and_udp")
	})
}

func TestCamelToSnakeWithDelimiter(t *testing.T) {
	convey.Convey("TestCamelToSnakeWithDelimiter", t, func() {
		convey.So(str.CamelToSnakeWithDelimiter("", ""), convey.ShouldEqual, "")
		convey.So(str.CamelToSnakeWithDelimiter("camelToSnake", ""), convey.ShouldEqual, "camel_to_snake")
		convey.So(str.CamelToSnakeWithDelimiter("camelToSnake", " "), convey.ShouldEqual, "camel_to_snake")
		convey.So(str.CamelToSnakeWithDelimiter("camelToSnake", "_"), convey.ShouldEqual, "camel_to_snake")
		convey.So(str.CamelToSnakeWithDelimiter("camelToSnake", "@"), convey.ShouldEqual, "camel@to@snake")
		convey.So(str.CamelToSnakeWithDelimiter("CamelToSnake", "_"), convey.ShouldEqual, "camel_to_snake")
		convey.So(str.CamelToSnakeWithDelimiter("CamelToSnake", "@"), convey.ShouldEqual, "camel@to@snake")
		convey.So(str.CamelToSnakeWithDelimiter("NetProtocolTCPAndUDP", "_"), convey.ShouldEqual, "net_protocol_tcp_and_udp")
		convey.So(str.CamelToSnakeWithDelimiter("NetProtocolTCPAndUDP", "="), convey.ShouldEqual, "net=protocol=tcp=and=udp")
	})
}

func TestSnakeToCamelWithInitialismList(t *testing.T) {
	convey.Convey("TestSnakeToCamelWithInitialismList", t, func() {
		convey.So(str.SnakeToCamelWithInitialismList("", true), convey.ShouldEqual, "")
		convey.So(str.SnakeToCamelWithInitialismList("snake_to_camel_with_initializes", true), convey.ShouldEqual, "SnakeToCamelWithInitializes")
		convey.So(str.SnakeToCamelWithInitialismList("snake_to_camel_with_initializes", false), convey.ShouldEqual, "snakeToCamelWithInitializes")
		convey.So(str.SnakeToCamelWithInitialismList("net_protocol_tcp_and_udp", true), convey.ShouldEqual, "NetProtocolTCPAndUDP")
		convey.So(str.SnakeToCamelWithInitialismList("net_protocol_tcp_and_udp", false), convey.ShouldEqual, "netProtocolTCPAndUDP")
		convey.So(str.SnakeToCamelWithInitialismList("net_protocol_tcp_and_udp", false, "UDP"), convey.ShouldEqual, "netProtocolTcpAndUDP")
		convey.So(str.SnakeToCamelWithInitialismList("net_protocol_tcp_and_udp", false, "UDP", "TCP"), convey.ShouldEqual, "netProtocolTCPAndUDP")
	})
}

func TestSnakeToCamelWithDefaultInitializes(t *testing.T) {
	convey.Convey("TestSnakeToCamelWithDefaultInitializes", t, func() {
		convey.So(str.SnakeToCamelWithDefaultInitializes("", true), convey.ShouldEqual, "")
		convey.So(str.SnakeToCamelWithDefaultInitializes("snake_to_camel_with_initializes", true), convey.ShouldEqual, "SnakeToCamelWithInitializes")
		convey.So(str.SnakeToCamelWithDefaultInitializes("snake_to_camel_with_initializes", false), convey.ShouldEqual, "snakeToCamelWithInitializes")
		convey.So(str.SnakeToCamelWithDefaultInitializes("net_protocol_tcp_and_udp", true), convey.ShouldEqual, "NetProtocolTCPAndUDP")
		convey.So(str.SnakeToCamelWithDefaultInitializes("net_protocol_tcp_and_udp", false), convey.ShouldEqual, "netProtocolTCPAndUDP")
	})
}

func TestSnakeToCamelWithInitializes(t *testing.T) {
	convey.Convey("TestSnakeToCamelWithInitializes", t, func() {
		convey.So(str.SnakeToCamelWithInitializes("", true, nil), convey.ShouldEqual, "")
		convey.So(str.SnakeToCamelWithInitializes("snake_to_camel_with_initializes", true, nil), convey.ShouldEqual, "SnakeToCamelWithInitializes")
		convey.So(str.SnakeToCamelWithInitializes("snake_to_camel_with_initializes", false, nil), convey.ShouldEqual, "snakeToCamelWithInitializes")
		convey.So(str.SnakeToCamelWithInitializes("net_protocol_tcp_and_udp", true, nil), convey.ShouldEqual, "NetProtocolTCPAndUDP")
		convey.So(str.SnakeToCamelWithInitializes("net_protocol_tcp_and_udp", true, map[string]bool{"TCP": true}), convey.ShouldEqual, "NetProtocolTCPAndUdp")
		convey.So(str.SnakeToCamelWithInitializes("net_protocol_tcp_and_udp", false, nil), convey.ShouldEqual, "netProtocolTCPAndUDP")
		convey.So(str.SnakeToCamelWithInitializes("net_protocol_tcp_and_udp", false, map[string]bool{"TCP": true, "UDP": false}), convey.ShouldEqual, "netProtocolTCPAndUdp")
		convey.So(str.SnakeToCamelWithInitializes("net_protocol_tcp_and_udp", false, map[string]bool{"TCP": true, "UDP": true}), convey.ShouldEqual, "netProtocolTCPAndUDP")
	})
}
