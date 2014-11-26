package ftp

import "fmt"

// NewConsoleLogger creates a new logger that writes all messages sent and
// received to the console using fmt.Print. It can be used with ConnectLogging
// which might be helpful during debugging.
// If this is used in a GUI, note that ALL messages on the control connection
// are logged, including the password for the PASS command. You might not want
// the clear-text password to appear in the GUI.
func NewConsoleLogger() Logger {
	return consoleLogger{}
}

type consoleLogger struct{}

func (consoleLogger) SentFTP(msg []byte, err error) {
	write("--->", msg, err)
}

func (consoleLogger) ReceivedFTP(response []byte, err error) {
	write("<---", response, err)
}

func write(arrow string, msg []byte, err error) {
	fmt.Print(arrow, " ")
	if err != nil {
		fmt.Print("ERROR:", err.Error())
	} else {
		fmt.Print(string(msg))
	}
}
