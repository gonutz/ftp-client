FTP Client Library for Go
=========================

This is a Go implementation of the FTP protocol (RFC 959). It implements only the client part of the specification and supports the most common actions, including binary file transfer to and from a server.
The interface conforms to go conventions. For example the upload and download functions use io.Reader and io.Writer. This makes them flexible and easy to use with the rest of the Go standard library.
See the [documentation](https://godoc.org/github.com/gonutz/ftp-client/ftp) for details.

# Example

```Go
package main

import (
	"bytes"
	"fmt"
	"github.com/gonutz/ftp-client/ftp"
)

func main() {
	conn, err := ftp.Connect("some.ftp.host", 21)
	if err != nil {
		fmt.Println("unable to connect, error:", err)
		return
	}
	defer conn.Close()

	err = conn.Login("user", "password")
	if err != nil {
		fmt.Println("login failed, error:", err)
		return
	}
	defer conn.Quit()

	source := bytes.NewBuffer([]byte("This is the file content."))
	err = conn.Upload(source, "/example_upload.txt")
	if err != nil {
		fmt.Println("unable to upload file, error:", err)
	}
}
```

This example connects to an FTP server and logs in with the user and password. It then uploads the string `This is the file content.` to the file `example_upload.txt`. Note that the use of `defer` makes sure that the connection is closed after usage.