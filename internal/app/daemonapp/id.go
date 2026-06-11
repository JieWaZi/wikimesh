package daemonapp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read 理论上很少失败；保底返回可识别 ID，避免调用点再分支处理。
		return fmt.Sprintf("%s_fallback", prefix)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
