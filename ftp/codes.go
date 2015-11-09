package ftp

type responseCode string

const (
	commandOk                        responseCode = "200"
	systemStatusOrHelpReply                       = "211"
	directoryStatus                               = "212"
	fileStatus                                    = "213"
	helpMessage                                   = "214"
	systemName                                    = "215"
	serviceReadyForNewUser                        = "220"
	serviceClosingControlConnection               = "221"
	noTransferInProgress                          = "225"
	closingDataConnection                         = "226"
	enteringPassiveMode                           = "227"
	userLoggedIn_Proceed                          = "230"
	fileActionCompleted                           = "250"
	pathNameCreated                               = "257"
	userNameOK_NeedPassword                       = "331"
	needAccountForLogin                           = "332"
	fileActionPending                             = "350"
	connectionClosed_TransferAborter              = "426"
)

func (c responseCode) ok() bool {
	if len(c) != 3 {
		return false
	}
	return c[0] == '1' || c[0] == '2'
}
