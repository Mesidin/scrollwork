package protocol

import (
	"encoding/json"
	"strings"
)

// PasswordUpdated is the system line sent after an account password change.
// The launcher matches it exactly so saved bookmarks can be rewritten locally.
const PasswordUpdated = "Password updated."

// passwordChangePrefix marks a command line that carries both passwords.
// It is not a typed verb. Clients send it only after a masked prompt, and
// they do not copy it into the scroll or the input history.
const passwordChangePrefix = "\x1epassword-change\x1e"

// EncodePasswordChange packs the current and new passwords into one command
// line. Either password may contain spaces.
func EncodePasswordChange(oldPass, newPass string) (string, error) {
	b, err := json.Marshal([2]string{oldPass, newPass})
	if err != nil {
		return "", err
	}
	return passwordChangePrefix + string(b), nil
}

// DecodePasswordChange reports the passwords carried by EncodePasswordChange.
func DecodePasswordChange(line string) (oldPass, newPass string, ok bool) {
	if !strings.HasPrefix(line, passwordChangePrefix) {
		return "", "", false
	}
	var pair [2]string
	if err := json.Unmarshal([]byte(line[len(passwordChangePrefix):]), &pair); err != nil {
		return "", "", false
	}
	return pair[0], pair[1], true
}

// IsPasswordChange reports whether line is an encoded password change.
func IsPasswordChange(line string) bool {
	return strings.HasPrefix(line, passwordChangePrefix)
}
