package auth

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func protectPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return errors.New("connection storage must not be a link or special file")
	}
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	existing, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := existing.Owner()
	if err != nil {
		return err
	}
	if !storageOwnerAllowed(owner, user.User.Sid, token.IsElevated()) {
		return errors.New("connection storage must be owned by the current user")
	}
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + inherit + ";FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	securityInfo := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	var newOwner *windows.SID
	if !owner.Equals(user.User.Sid) {
		// Elevated processes can create Administrators-owned files; normalize them to the individual user.
		securityInfo |= windows.OWNER_SECURITY_INFORMATION
		newOwner = user.User.Sid
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, securityInfo, newOwner, nil, dacl, nil)
}

func storageOwnerAllowed(owner, user *windows.SID, elevated bool) bool {
	return owner.Equals(user) || (elevated && owner.IsWellKnown(windows.WinBuiltinAdministratorsSid))
}

func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}
