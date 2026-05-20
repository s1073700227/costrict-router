package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"regexp"
	"strconv"
	"time"
)

var machineCodePattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func GenerateMachineCode() string {
	seed := GenerateUUIDLike() + strconv.FormatInt(time.Now().UnixNano(), 10)
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func ValidMachineCode(value string) bool {
	return machineCodePattern.MatchString(value)
}

func GenerateState() string {
	return randomBase36(12) + strconv.FormatInt(time.Now().UnixMilli(), 36)
}

func GenerateUUIDLike() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

func randomBase36(length int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			out[i] = alphabet[time.Now().UnixNano()%int64(len(alphabet))]
			continue
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out)
}
