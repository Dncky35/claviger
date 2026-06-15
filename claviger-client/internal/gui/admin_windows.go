//go:build windows

package gui

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnsureAdmin checks for admin rights. If missing, it triggers the Windows UAC Prompt!
func EnsureAdmin() {
	if !isAdmin() {
		exe, _ := os.Executable()

		verb, _ := windows.UTF16PtrFromString("runas")
		file, _ := windows.UTF16PtrFromString(exe)

		// This native Windows API call triggers the UAC Shield Yes/No prompt
		err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_NORMAL)

		// Whether they click "Yes" (re-launches as admin) or "No" (aborts),
		// this original standard-user process must die quietly.
		if err == nil {
			os.Exit(0)
		}
	}
}

// isAdmin checks if the current process actually holds the rights
func isAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID, windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}
