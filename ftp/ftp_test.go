package ftp

import "testing"

func TestCompleteResponseHasCodeThenSpaceAndNewLine(t *testing.T) {
	checkCompleteResponse(t, "123 optional text\r\n")
	checkCompleteResponse(t, "456 \r\n")
	checkIncompleteResponse(t, "")
	checkIncompleteResponse(t, "12")
	checkIncompleteResponse(t, "456")
	checkIncompleteResponse(t, "789 ")
	checkIncompleteResponse(t, "321 \r")
	checkIncompleteResponse(t, "564 \n")
	checkIncompleteResponse(t, "98 \r\n")
}

func TestCompleteMultiLineResponseEndsWithStartCode(t *testing.T) {
	checkCompleteResponse(t, "123-some text\r\n123 \r\n")
	checkCompleteResponse(t, "123-\r\nline2\r\n123no space\r\n123 \r\n")
	checkCompleteResponse(t, "123-some text\r\nnext line\r\n123 \r\n")
	checkIncompleteResponse(t, "123-some text\r\n")
	checkIncompleteResponse(t, "123-some text\r\n123\r\n")
	checkIncompleteResponse(t, "123-some text\r\n123 \r")
	checkIncompleteResponse(t, "123-some text\r\n123no space\r\n")
	checkIncompleteResponse(t, "123-STAT\r\n")
}

func TestPASVresponseHasHostAndPort(t *testing.T) {
	checkPASVaddress(t, "227 passive mode (0,0,0,0,0,0) \r\n", "0.0.0.0:0")
	checkPASVaddress(t, "227 passive mode (127,12,0,1,1,2) \r\n", "127.12.0.1:258")
}

func TestHelpStringsAreStrippedOfControlSymbols(t *testing.T) {
	checkHelp(t,
		"214 some help text\r\n",
		"some help text")
	checkHelp(t,
		"214-multi line\r\nhelp\r\n214 over\r\n",
		"multi line\r\nhelp\r\nover")
	checkHelp(t,
		"214-last line is empty\r\nthus stripped\r\n214 ",
		"last line is empty\r\nthus stripped")
}

func TestPathComesInQuotes(t *testing.T) {
	checkExtractedPath(t, "257 \"path\"\r\n", "path")
	checkExtractedPath(t, "257-\"path\"\r\n257 \r\n", "path")
}

// test helpers

func checkCompleteResponse(t *testing.T, msg string) {
	ok := isCompleteResponse([]byte(msg))
	if !ok {
		t.Errorf("expected complete but was not: %v", msg)
	}
}

func checkIncompleteResponse(t *testing.T, msg string) {
	ok := isCompleteResponse([]byte(msg))
	if ok {
		t.Errorf("expected incomplete but was not: %v", msg)
	}
}

func checkPASVaddress(t *testing.T, msg, expectedAddr string) {
	addr, err := getAddressOfPasvResponse([]byte(msg))
	if err != nil {
		t.Errorf("got error %v", err.Error())
	}
	if addr != expectedAddr {
		t.Errorf("PASV expected %v but was %v", expectedAddr, addr)
	}
}

func checkHelp(t *testing.T, resp, expected string) {
	help := removeControlSymbols([]byte(resp))
	if help != expected {
		t.Errorf("expected help:\n'%v'\nbut was:\n'%v'", expected, help)
	}
}

func checkExtractedPath(t *testing.T, resp, expected string) {
	path, err := getPathFromResponse([]byte(resp))
	if err != nil {
		t.Errorf("got error %v", err.Error())
	}
	if path != expected {
		t.Errorf("expected path '%v' but got '%v'", expected, path)
	}
}
