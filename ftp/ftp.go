package ftp

// TODO commands in RFC 959
// 'x' = done, '-' = not to be implemented
// Connection Establishment x
// USER x
// PASS x
// ACCT   TODO
// CWD  x
// CDUP x
// SMNT x
// REIN x
// QUIT x
// PORT -
// PASV x
// MODE -
// TYPE x
// STRU -
// ALLO -
// REST -
// STOR x
// STOU x
// RETR x
// LIST x
// NLST x
// APPE x
// RNFR x
// RNTO x
// DELE x
// RMD  x
// MKD  x
// PWD  x
// ABOR x
// SYST x
// STAT x
// HELP x
// SITE   TODO
// NOOP x

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

type Connection struct {
	conn         net.Conn
	logger       Logger
	transferType transferType
}

type Logger interface {
	SentFTP(msg []byte, err error)
	ReceivedFTP(response []byte, err error)
}

type transferType string

const (
	transferASCII  transferType = "ASCII"
	transferBinary              = "binary"
)

func Connect(host string, port uint16) (*Connection, error) {
	return ConnectLogging(host, port, nil)
}

func ConnectLogging(host string, port uint16, logger Logger) (*Connection, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return newConnection(conn, logger)
}

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

func isMultiLineResponse(msg []byte) bool {
	return len(msg) >= 4 && msg[3] == '-'
}

func isCompleteMultiLineResponse(msg []byte) bool {
	return isMultiLineResponse(msg) && lastLineEndsInSameCodeAsFirstLine(msg)
}

func lastLineEndsInSameCodeAsFirstLine(msg []byte) bool {
	// msg should end in \r\n so last splitting at \r\n creates an empty string
	// at the end. This makes the actual last line of the msg the second last
	// split line.
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

// If no password is required the FTP server will respond accordingly and no
// password will be sent. In this case just pass an empty string for the password.
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

func (c *Connection) execute(expectedCode responseCode, args ...string) error {
	_, err := c.executeGetResponse(expectedCode, args...)
	return err
}

func (c *Connection) ChangeWorkingDirTo(path string) error {
	return c.execute(fileActionCompleted, "CWD", path)
}

func (c *Connection) ChangeDirUp() error {
	return c.execute(commandOk, "CDUP")
}

func (c *Connection) StructureMount(path string) error {
	return c.execute(fileActionCompleted, "SMNT", path)
}

func (c *Connection) Reinitialize() error {
	return c.execute(serviceReadyForNewUser, "REIN")
}

func (c *Connection) Quit() error {
	return c.execute(serviceClosingControlConnection, "QUIT")
}

func (c *Connection) RenameFromTo(from, to string) error {
	err := c.execute(fileActionPending, "RNFR", from)
	if err != nil {
		return err
	}
	return c.execute(fileActionCompleted, "RNTO", to)
}

func (c *Connection) Delete(path string) error {
	return c.execute(fileActionCompleted, "DELE", path)
}

func (c *Connection) MakeDirectory(path string) (string, error) {
	resp, err := c.executeGetResponse(pathNameCreated, "MKD", path)
	if err != nil {
		return "", err
	}
	return getPathFromResponse(resp)
}

func (c *Connection) RemoveDirectory(path string) error {
	return c.execute(fileActionCompleted, "RMD", path)
}

func (c *Connection) NoOperation() error {
	return c.execute(commandOk, "NOOP")
}

func (c *Connection) Help() (string, error) {
	return c.HelpAbout("")
}

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

func (c *Connection) Status() (StatusType, string, error) {
	return c.StatusOf("")
}

type StatusType string

const (
	GeneralStatus   StatusType = "status"
	FileStatus                 = "file status"
	DirectoryStatus            = "directory status"
)

func (c *Connection) StatusOf(path string) (StatusType, string, error) {
	err := c.sendWithoutEmptyString("STAT", path)
	if err != nil {
		return "", "", err
	}
	resp, code, err := c.receive()
	if err != nil {
		return "", "", err
	}
	if code == systemStatusOrHelpReply {
		return GeneralStatus, removeControlSymbols(resp), nil
	}
	if code == directoryStatus {
		return DirectoryStatus, removeControlSymbols(resp), nil
	}
	if code == fileStatus {
		return FileStatus, removeControlSymbols(resp), nil
	}
	return "", "", errorMessage("STAT", resp)
}

func (c *Connection) System() (string, error) {
	resp, err := c.executeGetResponse(systemName, "SYST")
	if err != nil {
		return "", err
	}
	return removeControlSymbols(resp), nil
}

func (c *Connection) PrintWorkingDirectory() (string, error) {
	resp, err := c.executeGetResponse(pathNameCreated, "PWD")
	if err != nil {
		return "", err
	}
	return getPathFromResponse(resp)
}

func (c *Connection) Abort() error {
	resp, code, err := c.sendAndReceive("ABOR")
	if err != nil {
		return err
	}
	if code == noTransferInProgress || code == closingDataConnection {
		return nil
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

func (c *Connection) SetASCIITransfer() error {
	return c.setTransferTypeTo(transferASCII, "A")
}

func (c *Connection) SetBinaryTransfer() error {
	return c.setTransferTypeTo(transferBinary, "I")
}

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

func (c *Connection) ListFiles() (string, error) {
	return c.ListFilesIn("")
}

func (c *Connection) ListFilesIn(path string) (string, error) {
	return c.readListCommandData("LIST", path)
}

func (c *Connection) ListFileNames() ([]string, error) {
	return c.ListFileNamesIn("")
}

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
	err := c.SetASCIITransfer()
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
	if !code.success() {
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
	if !code.success() {
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

func (c *Connection) Download(path string, dest io.Writer) error {
	err := c.SetBinaryTransfer()
	if err != nil {
		return err
	}
	dataConn, err := c.enterPassiveMode()
	if err != nil {
		return err
	}
	err = c.send("RETR", path)
	if err != nil {
		return err
	}
	resp, code, err := c.receive()
	if err != nil {
		return err
	}
	if !code.success() {
		return errorMessage("RETR", resp)
	}
	_, err = io.Copy(dest, dataConn)
	if err != nil {
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
	if !code.success() {
		return errorMessage("RETR", resp)
	}
	return nil
}

func (c *Connection) Upload(source io.Reader, path string) error {
	return c.upload("STOR", path, source)
}

func (c *Connection) UploadUnique(source io.Reader) error {
	return c.upload("STOU", "", source)
}

func (c *Connection) Append(source io.Reader, path string) error {
	return c.upload("APPE", path, source)
}

func (c *Connection) upload(cmd, path string, source io.Reader) error {
	err := c.SetBinaryTransfer()
	if err != nil {
		return err
	}
	dataConn, err := c.enterPassiveMode()
	if err != nil {
		return err
	}
	err = c.sendWithoutEmptyString(cmd, path)
	if err != nil {
		return err
	}
	resp, code, err := c.receive()
	if err != nil {
		return err
	}
	if !code.success() {
		return errorMessage(cmd, resp)
	}
	_, err = io.Copy(dataConn, source)
	if err != nil {
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
	if !code.success() {
		return errorMessage(cmd, resp)
	}
	return nil
}
