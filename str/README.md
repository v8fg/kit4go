# string utils

> `import "github.com/v8fg/kit4go/str"`

## bytes-string converter

- `str.BytesToString` bytes to string
- `str.StringToBytes` string to bytes

## check

- `str.CharIsAlphabet` checks whether the char(byte) is a letter
- `str.CharIsNumber` checks whether the char is a number
- `str.ContainsAll` checks whether the string contains all the given characters
- `str.ContainsAny` checks whether the string contains any one of the given characters
- `str.StartWithAny` checks whether the string starts with any one of the given characters
- `str.EndWithAny` checks whether the string ends with any one of the given characters
- `str.EqualIgnoreCase` checks whether the strings are equal, ignore the case
- `str.IsBlank` checks whether the string is empty or only contain the space character
- `str.IsEmpty` checks whether the string length equal to zero

## convert

- `str.Lower` converts a string to the lowercase
- `str.Upper` converts a string to the uppercase
- `str.Titles` converts a string to the title format
- `str.Quote` converts a string to the quote format
- `str.Unquote` converts a string to the unquoted format
- `str.Camel` converts a string to the camel case, can specify the delimiter(rune character)
- `str.CamelToSnake` converts a string to the camel case, with the default delimiter `_`
- `str.CamelToSnakeWithDelimiter` converts a string to the snake case from camel case, with given delimiter
- `str.SnakeToCamel` converts a string from snake case to camel case
- `str.SnakeToCamelWithDefaultInitializes` converts a string from snake to camel with default initializes
- `str.SnakeToCamelWithInitialismList` converts a string from snake to camel with your given initialism list
- `str.SnakeToCamelWithInitializes` converts a string from snake to camel with your given initializes mapping
