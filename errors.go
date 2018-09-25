package main

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
)

type ProtocolError struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Reason  *string `json:"reason"`
}

func (e ProtocolError) Error() string {
	if e.Reason != nil {
		return fmt.Sprintf("%s: %s", e.Message, *e.Reason)
	}
	return e.Message
}

func BadRequest(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    1,
		Message: message,
		Reason:  reason,
	}
}

func ClientNotConnected(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    2,
		Message: message,
		Reason:  reason,
	}
}

func LocalError(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    2,
		Message: message,
		Reason:  reason,
	}
}

func InternalError(message string, reason *string) ProtocolError {
	return ProtocolError{
		Code:    3,
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
		Code:    5,
		Message: message,
		Reason:  reason,
	}
}

func ApplyReason(fn func(message string, reason *string) ProtocolError, message string, err error) *ProtocolError {
	spew.Dump("MMMMMM", err)
	var reason *string
	if err != nil {
		tmp := err.Error()
		reason = &tmp
	}
	e := fn(message, reason)
	return &e
}
