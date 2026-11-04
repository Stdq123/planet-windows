//go:build windows && amd64

package updates

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unicode/utf16"
)

func createDownloadTemp(dir string) (*os.File, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	path := filepath.Join(dir, ".planet-update-"+hex.EncodeToString(b)+".part")
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE, 0, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(h), path), nil
}
func publishDownload(f *os.File, path string) error {
	name := utf16.Encode([]rune(path))
	buf := make([]byte, 22+2*len(name))
	binary.LittleEndian.PutUint32(buf[16:20], uint32(2*len(name)))
	for i, c := range name {
		binary.LittleEndian.PutUint16(buf[20+2*i:], c)
	}
	return windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileRenameInfo, &buf[0], uint32(len(buf)))
}
func discardDownload(f *os.File) error {
	flag := byte(1)
	return windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileDispositionInfo, &flag, 1)
}

func openDownloaded(path string) (*os.File, error) {
	return openDownloadForAccess(path, false)
}
func openDownloadForAccess(path string, remove bool) (*os.File, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	access := uint32(windows.GENERIC_READ)
	if remove {
		access |= windows.DELETE
	}
	h, e := windows.CreateFile(p, access, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, e
	}
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &info); e != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		windows.CloseHandle(h)
		return nil, errors.New("更新文件不是独立的普通文件")
	}
	return os.NewFile(uintptr(h), path), nil
}
func pinDownloadDirectory(dir string) (func(), error) {
	absolute, e := filepath.Abs(dir)
	if e != nil {
		return nil, e
	}
	handles := []windows.Handle{}
	close := func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}
	for path := absolute; ; path = filepath.Dir(path) {
		p, e := windows.UTF16PtrFromString(path)
		if e != nil {
			close()
			return nil, e
		}
		h, e := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if e != nil {
			close()
			return nil, e
		}
		handles = append(handles, h)
		var info windows.ByHandleFileInformation
		if e = windows.GetFileInformationByHandle(h, &info); e != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			close()
			return nil, errors.New("更新目录被重定向")
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	return close, nil
}
