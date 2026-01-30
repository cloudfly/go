package ngxparser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
)

type BlockSetting [][]interface{}

func validToken(b byte) bool {
	if b == ';' || b == '{' || b == '}' || b == '#' || b == '\n' {
		return true
	}
	return false
}

func inQuote(content []byte) bool {
	var (
		token byte
		in    bool
	)
	for i, b := range content {
		if b == '\'' || b == '"' {
			if token == 0 {
				token = b
				in = true
			} else if b == token {
				if content[i-1] != '\\' {
					token = 0
					in = false
				}
			}
		}
	}
	return in
}

func splitFunc(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if validToken(b) {
			return i + 1, data[:i+1], nil
		}
	}
	if !atEOF {
		return 0, nil, nil
	}
	ret := bytes.TrimSpace(data)
	return len(ret), ret, nil // return the left bytes
}

func newScanner(r io.Reader) *bufio.Scanner {
	buf := make([]byte, 1024*1024)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(buf, 1024*1024*128)
	scanner.Split(splitFunc)
	return scanner
}

// next return the scaned bytes and the end token.
// it return all bytes in scanner if fail to find token before EOF, and returned end token will be 0.
func next(scanner *bufio.Scanner) ([]byte, byte) {
	buf := make([]byte, 0, 100)
	for scanner.Scan() {
		tokens := scanner.Bytes()
		if len(tokens) == 0 {
			return tokens, 0
		}
		if validToken(tokens[len(tokens)-1]) {
			buf = append(buf, tokens...)
			if inQuote(buf) && tokens[len(tokens)-1] != '\n' {
				continue
			}
			return buf[:len(buf)-1], buf[len(buf)-1]
		}
		return tokens, 0
	}
	return []byte{}, 0
}

// find the given tokens, return the scanned bytes before token.
// it return error if fail to find token before EOF.
func findToken(scanner *bufio.Scanner, tokens ...byte) ([]byte, byte, error) {
	buf := make([]byte, 0, 100)
	for {
		content, t := next(scanner)
		buf = append(buf, content...)
		for _, token := range tokens {
			if t == token {
				return buf, t, nil
			}
		}
		if t == 0 {
			return buf, t, io.EOF
		}
	}
}

func toArgv(s string) []string {
	const (
		InArg = iota
		InArgQuote
		OutOfArg
	)
	currentState := OutOfArg
	currentQuoteChar := "\x00" // to distinguish between ' and " quotations
	// this allows to use "foo'bar"
	currentArg := ""
	argv := []string{}

	isQuote := func(c string) bool {
		return c == `"` || c == `'`
	}

	isEscape := func(c string) bool {
		return c == `\`
	}

	isWhitespace := func(c string) bool {
		return c == " " || c == "\t"
	}

	L := len(s)
	for i := 0; i < L; i++ {
		c := s[i : i+1]

		//fmt.Printf("c %s state %v arg %s argv %v i %d\n", c, currentState, currentArg, args, i)
		if isQuote(c) {
			switch currentState {
			case OutOfArg:
				currentArg = ""
				fallthrough
			case InArg:
				currentState = InArgQuote
				currentQuoteChar = c

			case InArgQuote:
				if c == currentQuoteChar {
					currentState = InArg
				} else {
					currentArg += c
				}
			}

		} else if isWhitespace(c) {
			switch currentState {
			case InArg:
				argv = append(argv, currentArg)
				currentState = OutOfArg
			case InArgQuote:
				currentArg += c
			case OutOfArg:
				// nothing
			}

		} else if isEscape(c) {
			switch currentState {
			case OutOfArg:
				currentArg = ""
				currentState = InArg
				fallthrough
			case InArg:
				fallthrough
			case InArgQuote:
				if i == L-1 {
					if runtime.GOOS == "windows" {
						// just add \ to end for windows
						currentArg += c
					} else {
						panic("Escape character at end string")
					}
				} else {
					if runtime.GOOS == "windows" {
						peek := s[i+1 : i+2]
						if peek != `"` {
							currentArg += c
						}
					} else {
						i++
						c = s[i : i+1]
						currentArg += c
					}
				}
			}
		} else {
			switch currentState {
			case InArg, InArgQuote:
				currentArg += c

			case OutOfArg:
				currentArg = ""
				currentArg += c
				currentState = InArg
			}
		}
	}

	if currentState == InArg {
		argv = append(argv, currentArg)
	} else if currentState == InArgQuote {
		panic("Starting quote has no ending quote.")
	}

	return argv
}

func tokenFields(tokens []byte) []interface{} {
	if len(tokens) == 0 {
		return []interface{}{}
	}
	fields := toArgv(string(tokens))
	items := make([]interface{}, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		items = append(items, fields[i])
	}
	return items
}

func isLua(field string) bool {
	if field == "content_by_lua_block" ||
		field == "rewrite_by_lua_block" ||
		field == "access_by_lua_block" {
		return true
	}
	return false
}

func findBlockEnd(scanner *bufio.Scanner) ([]byte, error) {
	buf := make([]byte, 0, 1024)
	bracket := 0
	for {
		content, token, err := findToken(scanner, '{', '}')
		if err != nil {
			return nil, errors.New("block do not ended with '}'")
		}
		buf = append(buf, content...)

		if token == '{' {
			bracket++
			buf = append(buf, '{')
		} else if token == '}' {
			if bracket == 0 {
				return buf, nil
			}
			buf = append(buf, '}')
			bracket--
		}
	}
}

func parseBlock(scanner *bufio.Scanner) (BlockSetting, error) {
	block := make(BlockSetting, 0, 100)
	bracket := 0

	buf := make([]byte, 0, 1024)

	for {
		tokens, token := next(scanner)

		switch token {
		case ';':
			block = append(block, tokenFields(append(buf, tokens...)))
			buf = buf[:0] // clear buffer
		case '{': // sub block started
			bracket++
			fields := tokenFields(append(buf, tokens...))
			buf = buf[:0] // clear buffer

			if len(fields) > 0 && isLua(fields[0].(string)) { // is a lua code block
				buf, err := findBlockEnd(scanner)
				if err != nil {
					return nil, err
				}
				fields = append(fields, string(buf))

			} else { // is a normal block
				subBlock, err := parseBlock(scanner) // scanner starting from new block without the first byte '{'
				if err != nil {
					return nil, err
				}
				fields = append(fields, subBlock)
			}

			block = append(block, fields)
		case '}':
			if bracket == 0 { // end of this block
				return block, nil
			}
			bracket--
		case '#':
			buf = append(buf, tokens...)
			// find end of line
			if _, _, err := findToken(scanner, '\n'); err != nil && err != io.EOF {
				return nil, err
			}
		case '\n':
			// add this tokens into buffer, some instruction(such as 'log_format') having multiple lines arguments
			buf = append(buf, tokens...)
		case 0: // end of file
			return block, nil
		}
	}
}

func Parse(file io.Reader) {
	scanner := newScanner(file)
	block, err := parseBlock(scanner)
	if err != nil {
		panic(err)
	}
	content, err := json.Marshal(block)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", content)
}
