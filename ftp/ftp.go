/*
Package ftp implements the FTP client protocol as specified in RFC 959.
*/
package ftp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Connection is the network connection to an FTP server. The Connect functions
// return a *Connection which you have to Close after usage.
type Connection struct {
	conn         net.Conn
	logger       Logger
	transferType transferType
}

// Logger can be used to log the raw messages on the FTP control connection.
type Logger interface {
	// SentFTP is called after a message is sent to the FTP server on the control
	// connection. If an error occurred during the sent it is given to the Logger
	// as well.
	SentFTP(msg []byte, err error)
	// ReceivedFTP is called after a message is received from FTP server on the control
	// connection. If an error occurred while receiving it is given to the Logger as well.
	ReceivedFTP(response []byte, err error)
}

// Connect establishes a connection to the given host on the given port.
// The standard FTP port is 21.
func Connect(host string, port uint16) (*Connection, error) {
	return ConnectLogging(host, port, nil)
}

// ConnectLogging establishes a connection to the given host on the given port.
// All messages sent and reveived over the control connection are additionally
// passed to the given Logger.
// The standard FTP port is 21.
func ConnectLogging(host string, port uint16, logger Logger) (*Connection, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return newConnection(conn, logger)
}

// ConnectOn uses the given connection as an FTP control connection. This can be
// used for setting connection parameters like time-outs.
func ConnectOn(conn net.Conn) (*Connection, error) {
	return newConnection(conn, nil)
}

// ConnectLoggingOn uses the given connection as an FTP control connection. This
// can be used for setting connection parameters like time-outs. It also sets
// the logger.
func ConnectLoggingOn(conn net.Conn, logger Logger) (*Connection, error) {
	return newConnection(conn, logger)
}

type transferType string

const (
	transferASCII  transferType = "ASCII"
	transferBinary              = "binary"
)

func newConnection(conn net.Conn, logger Logger) (*Connection, error) {
	c := &Connection{conn, logger, transferASCII}
	resp, code, err := c.receive()
	if err != nil {
		return nil, err
	}
	if code != serviceReadyForNewUser {
		return nil, errorMessage("connect", resp)
	}
	return c, nil
}

func errorMessage(command string, response []byte) error {
	return errors.New("FTP server responded to " + command +
		" with error: " + string(response))
}

// Close closes the underlying TCP connection to the FTP server. Call this
// function when done. Closing does not send a QUIT message to the server
// so make sure to do that before-hand.
func (c *Connection) Close() {
	c.conn.Close()
}

func (c *Connection) send(words ...string) error {
	msg := strings.Join(words, " ") + "\r\n"
	_, err := c.conn.Write([]byte(msg))
	if c.logger != nil {
		c.logger.SentFTP([]byte(msg), err)
	}
	return err
}

func (c *Connection) sendWithoutEmptyString(cmd, arg string) error {
	if arg == "" {
		return c.send(cmd)
	}
	return c.send(cmd, arg)
}

// if the returned error is not nil then the response and the code are not meaningful
func (c *Connection) receive() (response []byte, code responseCode, e error) {
	msg, err := readResponse(c.conn)
	if c.logger != nil {
		c.logger.ReceivedFTP(msg, err)
	}
	return msg, extractCode(msg), err
}

func readResponse(conn net.Conn) ([]byte, error) {
	all := new(bytes.Buffer)
	buffer := make([]byte, 1024)
	done := false
	var err error
	for !done {
		done, err = readResponseInto(conn, buffer, all)
	}
	return all.Bytes(), err
}

func readResponseInto(con net.Conn, buf []byte, dest *bytes.Buffer) (done bool, err error) {
	n, err := con.Read(buf)
	if err != nil {
		return true, err
	}
	dest.Write(buf[:n])
	if isCompleteResponse(dest.Bytes()) {
		return true, nil
	}
	return false, nil
}

func isCompleteResponse(msg []byte) bool {
	return isCompleteSingleLineResponse(msg) || isCompleteMultiLineResponse(msg)
}

func isCompleteSingleLineResponse(msg []byte) bool {
	return isSingleLineResponse(msg) && endsInNewLine(msg)
}

func isSingleLineResponse(msg []byte) bool {
	return len(msg) >= 4 && msg[3] == ' '
}

func endsInNewLine(msg []byte) bool {
	l := len(msg)
	if l < 2 {
		return false
	}
	return msg[l-2] == '\r' && msg[l-1] == '\n'
}

func isCompleteMultiLineResponse(msg []byte) bool {
	return isMultiLineResponse(msg) && lastLineEndsInSameCodeAsFirstLine(msg)
}

func isMultiLineResponse(msg []byte) bool {
	return len(msg) >= 4 && msg[3] == '-'
}

func lastLineEndsInSameCodeAsFirstLine(msg []byte) bool {
	// msg should end in \r\n so splitting at \r\n creates an empty string
	// at the end. This makes the actual (non-empty) last line of the msg
	// the second last split part.
	lines := strings.Split(string(msg), "\r\n")
	if len(lines) < 3 {
		return false
	}
	first := lines[0]
	last := lines[len(lines)-2]
	if len(first) < 3 || len(last) < 4 {
		return false
	}
	codePlusSpace := first[:3] + " "
	return last[:4] == codePlusSpace
}

func extractCode(msg []byte) responseCode {
	if len(msg) <= 3 {
		return responseCode(msg)
	}
	return responseCode(msg[:3])
}

// Login sends the given user and, if required,  password to the FTP server.
// If no password is required (the FTP server will respond accordingly) no
// password will be sent. In this case just pass an empty string for the password.
// The FTP commands this sends are USER and (optionally) PASS.
func (c *Connection) Login(user, password string) error {
	err := c.send("USER", user)
	if err != nil {
		return err
	}
	resp, code, err := c.receive()
	if err != nil {
		return err
	}
	if code == userLoggedIn_Proceed {
		return nil
	}
	if code == userNameOK_NeedPassword {
		return c.execute(userLoggedIn_Proceed, "PASS", password)
	}
	return errorMessage("USER", resp)
}

func (c *Connection) execute(success responseCode, args ...string) error {
	_, err := c.executeGetResponse(success, args...)
	return err
}

func (c *Connection) executeGetResponse(expectedCode responseCode, args ...string) ([]byte, error) {
	err := c.send(args...)
	if err != nil {
		return nil, err
	}
	resp, code, err := c.receive()
	if err != nil {
		return nil, err
	}
	if code == expectedCode {
		return resp, nil
	}
	return nil, errorMessage(args[0], resp)
}

// ChangeWorkingDirTo sets the given path as the working directory. The path
// argument is sent as is so make sure to surround the string with quotes if
// needed.
// The FTP command this sends is CWD
func (c *Connection) ChangeWorkingDirTo(path string) error {
	return c.execute(fileActionCompleted, "CWD", path)
}

// ChangeDirUp moves the current working directory up one folder (like
// a 'cd ..' in the console).
// The FTP command this sends is CDUP.
func (c *Connection) ChangeDirUp() error {
	return c.execute(commandOk, "CDUP")
}

// StructureMount mounts the given path. The path argument is sent as is so
// make sure to surround the string with quotes if needed.
// The FTP command this sends is SMNT.
func (c *Connection) StructureMount(path string) error {
	return c.execute(fileActionCompleted, "SMNT", path)
}

// Reinitialize closes the current session and starts over again. You may want
// to Login again after this command.
// The FTP command this sends is REIN.
func (c *Connection) Reinitialize() error {
	return c.execute(serviceReadyForNewUser, "REIN")
}

// Quit closes the current FTP session. It does not however close the underlying
// TCP connection. For that you need to call Close once you are done.
// The FTP command this sends is QUIT.
func (c *Connection) Quit() error {
	return c.execute(serviceClosingControlConnection, "QUIT")
}

// RenameFromTo changes the name of a file (from) to the new name (to). The paths
// are sent as is so make sure to surround the strings with quotes if needed.
// The FTP commands this sends are RNFR and RNTO.
func (c *Connection) RenameFromTo(from, to string) error {
	err := c.execute(fileActionPending, "RNFR", from)
	if err != nil {
		return err
	}
	return c.execute(fileActionCompleted, "RNTO", to)
}

// Delete erases the given path from the FTP server. The path argument is sent as
// is so make sure to surround the string with quotes if needed.
// The FTP command this sends is DELE.
func (c *Connection) Delete(path string) error {
	return c.execute(fileActionCompleted, "DELE", path)
}

// MakeDirectory creates a new directory under the given path. Since this path
// may be relative to the current working directory and possibly not suited for
// a call to ChangeWorkingDirTo, on success (err is nil) the function returns
// the path to the newly created directory. The path is sent as is so make sure
// to surround the string with quotes if needed.
// The FTP command this sends is MKD.
func (c *Connection) MakeDirectory(path string) (string, error) {
	resp, err := c.executeGetResponse(pathNameCreated, "MKD", path)
	if err != nil {
		return "", err
	}
	return getPathFromResponse(resp)
}

// RemoveDirectory erases the directory under the given path. The path is sent
// as is so make sure to surround the string with quotes if needed.
// The FTP command this sends is RMD.
func (c *Connection) RemoveDirectory(path string) error {
	return c.execute(fileActionCompleted, "RMD", path)
}

// NoOperation sends a message to the FTP server and makes sure the repsonse is
// OK. This can be used as a kind of ping to see if the server is still responding.
// The FTP command this sends is NOOP.
func (c *Connection) NoOperation() error {
	return c.execute(commandOk, "NOOP")
}

// Help returns a human readable help message from the FTP server. This message
// does not contain any control codes.
// The FTP command this sends is HELP.
func (c *Connection) Help() (string, error) {
	return c.HelpAbout("")
}

// HelpAbout returns a human readable help message about the given topic from
// the FTP server. This message does not contain any control codes.
// The FTP command this sends is HELP.
func (c *Connection) HelpAbout(topic string) (string, error) {
	err := c.sendWithoutEmptyString("HELP", topic)
	if err != nil {
		return "", err
	}
	resp, code, err := c.receive()
	if err != nil {
		return "", err
	}
	if code == systemStatusOrHelpReply || code == helpMessage {
		return removeControlSymbols(resp), nil
	}
	return "", errorMessage("HELP", resp)
}

// removes the control codes and the last line feed (\r\n) from the response
func removeControlSymbols(resp []byte) string {
	noCodeOrNewLine := strings.TrimSuffix(string(resp[4:]), "\r\n")
	if isSingleLineResponse(resp) {
		return noCodeOrNewLine
	}
	lastLineStart := strings.LastIndex(noCodeOrNewLine, "\r\n")
	start := noCodeOrNewLine[:lastLineStart+2]
	end := noCodeOrNewLine[lastLineStart+6:]
	all := start + end
	return strings.TrimSuffix(all, "\r\n")
}

// Status returns general status information about the FTP server process. The
// resulting string does not contain any control codes.
// The FTP command this sends is STAT.
func (c *Connection) Status() (StatusType, string, error) {
	return c.StatusOf("")
}

// StatusOf returns status information about the given path. It behaves like
// ListFilesIn if called with the path. The resulting string does not contain
// any control codes.
// The FTP command this sends is STAT.
func (c *Connection) StatusOf(path string) (StatusType, string, error) {
	err := c.sendWithoutEmptyString("STAT", path)
	if err != nil {
		return "", "", err
	}
	resp, code, err := c.receive()
	if err != nil {
		return "", "", err
	}
	if typ, ok := statusTypeOfCode(code); ok {
		return typ, removeControlSymbols(resp), nil
	}
	return "", "", errorMessage("STAT", resp)
}

func statusTypeOfCode(code responseCode) (typ StatusType, ok bool) {
	if code == systemStatusOrHelpReply {
		return GeneralStatus, true
	}
	if code == directoryStatus {
		return DirectoryStatus, true
	}
	if code == fileStatus {
		return FileStatus, true
	}
	return "", false
}

// StatusType describes the result of a Status or StatusOf command.
type StatusType string

const (
	GeneralStatus   StatusType = "status"
	FileStatus                 = "file status"
	DirectoryStatus            = "directory status"
)

// System describes the system on which the FTP server is running. This may
// include the operating system and other information.
// The FTP command this sends is SYST.
func (c *Connection) System() (string, error) {
	resp, err := c.executeGetResponse(systemName, "SYST")
	if err != nil {
		return "", err
	}
	return removeControlSymbols(resp), nil
}

// PrintWorkingDirectory returns the current working directory.
// The FTP command this sends is PWD.
func (c *Connection) PrintWorkingDirectory() (string, error) {
	resp, err := c.executeGetResponse(pathNameCreated, "PWD")
	if err != nil {
		return "", err
	}
	return getPathFromResponse(resp)
}

// Abort aborts the currently running file transaction (if any). If no file
// transfer is being executed or if shutting down the data connection was
// successful, the returned error will be nil.
// The FTP command this sends is ABOR.
func (c *Connection) Abort() error {
	resp, code, err := c.sendAndReceive("ABOR")
	if err != nil {
		return err
	}
	if code == noTransferInProgress || code == closingDataConnection {
		return nil
	}
	if code == connectionClosed_TransferAborter {
		resp, code, err = c.receive()
		if err != nil {
			return err
		}
		if code == closingDataConnection {
			return nil
		}
		return errorMessage("ABOR", resp)
	}
	return errorMessage("ABOR", resp)
}

func (c *Connection) sendAndReceive(words ...string) ([]byte, responseCode, error) {
	err := c.send(words...)
	if err != nil {
		return nil, "", err
	}
	return c.receive()
}

var pathMatcher = regexp.MustCompile("[0-9][0-9][0-9][ |-]\"(.+)\".*\r\n")

func getPathFromResponse(resp []byte) (string, error) {
	if !pathMatcher.Match(resp) {
		return "", errorMessage("path extraction", resp)
	}
	matches := pathMatcher.FindSubmatch(resp)
	return string(matches[1]), nil
}

func (c *Connection) setASCIITransfer() error {
	return c.setTransferTypeTo(transferASCII, "A")
}

func (c *Connection) setBinaryTransfer() error {
	return c.setTransferTypeTo(transferBinary, "I")
}

// the symbol A is for ASCII and I is for binary data
func (c *Connection) setTransferTypeTo(t transferType, symbol string) error {
	if c.transferType == t {
		return nil
	}
	err := c.execute(commandOk, "TYPE", symbol)
	if err == nil {
		c.transferType = t
	}
	return err
}

// ListFiles returns detailed information about the current working directory.
// The result does not contain any control codes. The format of the result depends
// on the implementation of the server so no automatic parsing happens here.
// The FTP command this sends is LIST.
func (c *Connection) ListFiles() (string, error) {
	return c.ListFilesIn("")
}

// ListFilesIn returns detailed information about the given file or directory.
// The result does not contain any control codes. The format of the result depends
// on the implementation of the server so no automatic parsing happens here.
// The path is sent as is so make sure to surround the string with quotes if needed.
// The FTP command this sends is LIST.
func (c *Connection) ListFilesIn(path string) (string, error) {
	return c.readListCommandData("LIST", path)
}

// ListFileNames returns a list of file names in the current working directory.
// The FTP command this sends is NLST.
func (c *Connection) ListFileNames() ([]string, error) {
	return c.ListFileNamesIn("")
}

// ListFileNamesIn returns a list of file names in the given directory.
// The path is sent as is so make sure to surround the string with quotes if needed.
// The FTP command this sends is NLST.
func (c *Connection) ListFileNamesIn(path string) ([]string, error) {
	data, err := c.readListCommandData("NLST", path)
	if err != nil {
		return nil, err
	}
	return parseNLST(data), nil
}

func parseNLST(data string) []string {
	onlyNewLines := strings.Replace(data, "\r\n", "\n", -1)
	lines := strings.Split(onlyNewLines, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (c *Connection) readListCommandData(cmd, path string) (string, error) {
	err := c.setASCIITransfer()
	if err != nil {
		return "", err
	}
	dataConn, err := c.enterPassiveMode()
	if err != nil {
		return "", err
	}
	defer dataConn.Close()
	err = c.sendWithoutEmptyString(cmd, path)
	if err != nil {
		return "", err
	}
	resp, code, err := c.receive()
	if err != nil {
		return "", err
	}
	if !code.ok() {
		return "", errorMessage(cmd, resp)
	}
	data, err := ioutil.ReadAll(dataConn)
	if err != nil {
		return "", err
	}
	resp, code, err = c.receive()
	if err != nil {
		return "", err
	}
	if !code.ok() {
		return "", errorMessage(cmd, resp)
	}
	return string(data), nil
}

func (c *Connection) enterPassiveMode() (net.Conn, error) {
	resp, err := c.executeGetResponse(enteringPassiveMode, "PASV")
	if err != nil {
		return nil, err
	}
	addr, err := getAddressOfPasvResponse(resp)
	if err != nil {
		return nil, err
	}
	return net.Dial("tcp", addr)
}

var addrMatcher = regexp.MustCompile(
	".*\\(([0-9]+,[0-9]+,[0-9]+,[0-9]+),([0-9]+),([0-9]+)\\).*")

func getAddressOfPasvResponse(msg []byte) (string, error) {
	if !addrMatcher.Match(msg) {
		return "", errorMessage("address extraction", msg)
	}
	matches := addrMatcher.FindSubmatch(msg)
	ip := strings.Replace(string(matches[1]), ",", ".", -1)
	highPort, _ := strconv.Atoi(string(matches[2]))
	lowPort, _ := strconv.Atoi(string(matches[3]))
	port := strconv.Itoa(256*highPort + lowPort)
	return ip + ":" + port, nil
}

// Download writes the contents of the file at the given path into the given
// writer.
// It reads the file as binary data from the FTP server in passive mode.
// The FTP command this sends is RETR.
func (c *Connection) Download(path string, dest io.Writer) error {
	err := c.setBinaryTransfer()
	if err != nil {
		return err
	}
	dataConn, err := c.enterPassiveMode()
	if err != nil {
		return err
	}
	err = c.send("RETR", path)
	if err != nil {
		dataConn.Close()
		return err
	}
	resp, code, err := c.receive()
	if err != nil {
		dataConn.Close()
		return err
	}
	if !code.ok() {
		dataConn.Close()
		return errorMessage("RETR", resp)
	}
	_, err = io.Copy(dest, dataConn)
	if err != nil {
		dataConn.Close()
		return err
	}
	err = dataConn.Close()
	if err != nil {
		return err
	}
	resp, code, err = c.receive()
	if err != nil {
		return err
	}
	if !code.ok() {
		return errorMessage("RETR", resp)
	}
	return nil
}

// Upload writes the contents of the given source to a file at the given path
// on the server. If the file was there before, it is overwritten. Otherwise a
// new file is created.
// The file is written as binary in passive mode.
// The FTP command this sends is STOR.
func (c *Connection) Upload(source io.Reader, path string) error {
	return c.upload("STOR", path, source)
}

// UploadUnique writes the contents of the given source to a file at the given
// path on the server. If the file was there before, it is overwritten.
// Otherwise a new file is created.
// It file is written as binary in passive mode.
// The FTP command this sends is STOU.
func (c *Connection) UploadUnique(source io.Reader) error {
	return c.upload("STOU", "", source)
}

// Append appends the contents of the given source to a file at the given path
// on the server. If the file was there before, it is overwritten. Otherwise a
// new file is created.
// It file is written as binary in passive mode.
// The FTP command this sends is APPE.
func (c *Connection) Append(source io.Reader, path string) error {
	return c.upload("APPE", path, source)
}

func (c *Connection) upload(cmd, path string, source io.Reader) error {
	err := c.setBinaryTransfer()
	if err != nil {
		return err
	}
	dataConn, err := c.enterPassiveMode()
	if err != nil {
		return err
	}
	err = c.sendWithoutEmptyString(cmd, path)
	if err != nil {
		dataConn.Close()
		return err
	}
	resp, code, err := c.receive()
	if err != nil {
		dataConn.Close()
		return err
	}
	if !code.ok() {
		dataConn.Close()
		return errorMessage(cmd, resp)
	}
	_, err = io.Copy(dataConn, source)
	if err != nil {
		dataConn.Close()
		return err
	}
	err = dataConn.Close()
	if err != nil {
		return err
	}
	resp, code, err = c.receive()
	if err != nil {
		return err
	}
	if !code.ok() {
		return errorMessage(cmd, resp)
	}
	return nil
}
