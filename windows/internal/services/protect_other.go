//go:build !windows

package services

import "errors"

func protect([]byte) ([]byte, error)   { return nil, errors.New("Windows DPAPI required") }
func unprotect([]byte) ([]byte, error) { return nil, errors.New("Windows DPAPI required") }
