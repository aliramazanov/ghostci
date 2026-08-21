package runner

import "os"

const Disable = "GHOSTCI"

func Disabled() bool {
	switch os.Getenv(Disable) {
	case "0", "false":
		return true
	}

	return false
}
