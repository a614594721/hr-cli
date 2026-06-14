//go:build windows

package keychain

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

var (
	advapi32       = syscall.NewLazyDLL("advapi32.dll")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func platformGet(service, account string) (string, error) {
	targetName, err := syscall.UTF16PtrFromString(targetName(service, account))
	if err != nil {
		return "", err
	}
	var credPtr uintptr
	ret, _, callErr := procCredRead.Call(
		uintptr(unsafe.Pointer(targetName)),
		uintptr(credTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&credPtr)),
	)
	if ret == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.ERROR_NOT_FOUND {
			return "", ErrNotFound
		}
		return "", callErr
	}
	defer procCredFree.Call(credPtr)

	cred := (*credential)(unsafe.Pointer(credPtr))
	if cred.CredentialBlobSize == 0 || cred.CredentialBlob == nil {
		return "", nil
	}
	data := unsafe.Slice(cred.CredentialBlob, int(cred.CredentialBlobSize))
	value := make([]byte, len(data))
	copy(value, data)
	return string(value), nil
}

func platformSet(service, account, value string) error {
	target := targetName(service, account)
	targetName, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	userName, err := syscall.UTF16PtrFromString(account)
	if err != nil {
		return err
	}
	data := []byte(value)
	var blob *byte
	if len(data) > 0 {
		blob = &data[0]
	}
	cred := credential{
		Type:               credTypeGeneric,
		TargetName:         targetName,
		CredentialBlobSize: uint32(len(data)),
		CredentialBlob:     blob,
		Persist:            credPersistLocalMachine,
		UserName:           userName,
	}
	ret, _, callErr := procCredWrite.Call(uintptr(unsafe.Pointer(&cred)), 0)
	if ret == 0 {
		return fmt.Errorf("CredWriteW failed: %w", callErr)
	}
	return nil
}

func platformRemove(service, account string) error {
	targetName, err := syscall.UTF16PtrFromString(targetName(service, account))
	if err != nil {
		return err
	}
	ret, _, callErr := procCredDelete.Call(
		uintptr(unsafe.Pointer(targetName)),
		uintptr(credTypeGeneric),
		0,
	)
	if ret == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.ERROR_NOT_FOUND {
			return nil
		}
		return callErr
	}
	return nil
}

func targetName(service, account string) string {
	return service + ":" + account
}
