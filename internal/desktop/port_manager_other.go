//go:build !windows

package desktop

// TerminateProcessByPort 非 Windows 平台的空实现
func TerminateProcessByPort(port int) error {
	return nil
}

// CheckPortOccupied 非 Windows 平台的空实现
func CheckPortOccupied(port int) (int, error) {
	return -1, nil
}
