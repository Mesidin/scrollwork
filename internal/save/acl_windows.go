//go:build windows

package save

import (
	"runtime"

	"golang.org/x/sys/windows"
)

// restrictPrivateFile leaves the file readable by its owner only.
// WriteFile's 0600 mode does not become an ACL on Windows.
func restrictPrivateFile(path string) error {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;OW)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	runtime.KeepAlive(sd)
	return err
}
