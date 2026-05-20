package i18n

import (
	"os"
	"strings"
)

type Lang string

const (
	EN Lang = "en"
	ZH Lang = "zh"
)

var current = Detect()

func Detect() Lang {
	for _, key := range []string{"COSTRICT_ROUTER_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.ToLower(os.Getenv(key))
		if strings.HasPrefix(value, "zh") {
			return ZH
		}
		if strings.HasPrefix(value, "en") {
			return EN
		}
	}
	return EN
}

func IsZH() bool {
	return current == ZH
}

func T(en, zh string) string {
	if IsZH() {
		return zh
	}
	return en
}
