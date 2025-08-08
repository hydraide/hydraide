package elevation

import "os"

func IsElevated() bool {
	return os.Geteuid() == 0
}

func Hint(instanceName string) string {
	return "This command must be run as root or with sudo.\n" +
		"Please run: sudo hydraidectl service --instance " + instanceName
}
