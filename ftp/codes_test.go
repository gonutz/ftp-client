package ftp

import "testing"

func TestCodeOfLength3StartingWith1or2isSuccess(t *testing.T) {
	checkSuccess(t, "123")
	checkSuccess(t, "222")
	checkNotSuccess(t, "12")
	checkNotSuccess(t, "2")
	checkNotSuccess(t, "321")
}

func checkSuccess(t *testing.T, code string) {
	if !responseCode(code).success() {
		t.Errorf("%v expected success but was not", code)
	}
}

func checkNotSuccess(t *testing.T, code string) {
	if responseCode(code).success() {
		t.Errorf("%v expected no success but was success", code)
	}
}
