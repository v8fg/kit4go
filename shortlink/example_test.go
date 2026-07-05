package shortlink_test

import (
	"fmt"

	"github.com/v8fg/kit4go/shortlink"
)

func ExampleShortener() {
	s := shortlink.New(shortlink.WithCodeLength(6))
	code, _ := s.Generate("https://example.com/long/url")
	url, _ := s.Resolve(code)
	fmt.Println(len(code), url)
	// Output: 6 https://example.com/long/url
}

func ExampleIDShortener() {
	id := shortlink.NewIDShortener(shortlink.Alphabet, 0)
	c1 := id.Encode(61) // 61 in base62 = "z"
	c2 := id.Encode(62) // 62 in base62 = "10"
	fmt.Println(c1, c2)
	// Output: z 10
}
