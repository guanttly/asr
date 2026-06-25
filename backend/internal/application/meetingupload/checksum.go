package meetingupload

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

func sha256New() hash.Hash { return sha256.New() }

func hashHex(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }
