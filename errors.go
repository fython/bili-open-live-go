package biliopen

import "fmt"

// CommonErrorCode 公共错误码
type CommonErrorCode int

func (c CommonErrorCode) Desc() string {
	return errorCodeDescription[c]
}

func (c CommonErrorCode) String() string {
	return fmt.Sprintf("[%d] %s", c, errorCodeDescription[c])
}

// CommonError 公共错误
type CommonError struct {
	Code      CommonErrorCode
	Message   string
	RequestID string
}

func (e CommonError) Error() string {
	return fmt.Sprintf("%s: %s, request_id=%s", e.Code, e.Message, e.RequestID)
}
