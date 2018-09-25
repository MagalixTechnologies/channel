package channel

import (
	"fmt"
)

type ProtocolError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Reason  *string   `json:"reason"`
}

func (e ProtocolError) Error() string {
	if e.Reason != nil {
		return fmt.Sprintf("%s: %s", e.Message, *e.Reason)
	}
	return e.Message
}

type ErrorCode int

const (
	BadRequestCode         ErrorCode = 1
	ClientNotConnectedCode ErrorCode = 2
	LocalErrorCode         ErrorCode = 3
	InternalErrorCode      ErrorCode = 4
	NotFoundCode           ErrorCode = 5
	TimeoutCode            ErrorCode = 6
)

func BadRequest(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    BadRequestCode,
		Message: message,
		Reason:  reason,
	}
}

func ClientNotConnected(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    ClientNotConnectedCode,
		Message: message,
		Reason:  reason,
	}
}

func LocalError(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    LocalErrorCode,
		Message: message,
		Reason:  reason,
	}
}

func InternalError(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    InternalErrorCode,
		Message: message,
		Reason:  reason,
	}
}

func NotFound(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    4,
		Message: message,
		Reason:  reason,
	}
}

func Timeout(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    TimeoutCode,
		Message: message,
		Reason:  reason,
	}
}

func ApplyReason(fn func(message string, reason *string) ProtocolError, message string, err error) *ProtocolError {
	var reason *string
	if err != nil {
		tmp := err.Error()
		reason = &tmp
	}
	e := fn(message, reason)
	return &e
}
