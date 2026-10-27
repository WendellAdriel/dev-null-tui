package auth

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsStorageOwnerPolicy(t *testing.T) {
	user, err := windows.StringToSid("S-1-5-21-1-2-3-1001")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		owner    string
		elevated bool
		allowed  bool
	}{
		{"current user", user.String(), false, true},
		{"elevated current user", user.String(), true, true},
		{"elevated administrators", "S-1-5-32-544", true, true},
		{"unelevated administrators", "S-1-5-32-544", false, false},
		{"another user", "S-1-5-21-1-2-3-1002", true, false},
		{"system", "S-1-5-18", true, false},
		{"users group", "S-1-5-32-545", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, err := windows.StringToSid(test.owner)
			if err != nil {
				t.Fatal(err)
			}
			if got := storageOwnerAllowed(owner, user, test.elevated); got != test.allowed {
				t.Fatalf("owner allowed=%v, want %v", got, test.allowed)
			}
		})
	}
}

func TestWindowsStorageUsesCurrentUserOwnerAndProtectedACL(t *testing.T) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Dir: filepath.Join(t.TempDir(), "private")}
	if err := store.Prepare(); err != nil {
		t.Fatalf("prepare storage (user=%s elevated=%v): %v", user.User.Sid, token.IsElevated(), err)
	}
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(key, DefaultOrigin); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{store.Dir, filepath.Join(store.Dir, "connection.json")} {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		owner, _, err := sd.Owner()
		if err != nil {
			t.Fatal(err)
		}
		if !owner.Equals(user.User.Sid) {
			t.Fatalf("storage owner=%s, want %s", owner, user.User.Sid)
		}
		control, _, err := sd.Control()
		if err != nil {
			t.Fatal(err)
		}
		if control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatal("storage still inherits its DACL")
		}
		acl, _, err := sd.DACL()
		if err != nil {
			t.Fatal(err)
		}
		if acl == nil || acl.AceCount != 1 {
			t.Fatal("storage must grant access to only the current user")
		}
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, 0, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !strings.HasSuffix(sd.String(), ";;;"+user.User.Sid.String()+")") {
			t.Fatalf("unexpected storage ACL: %s", sd.String())
		}
	}
}
